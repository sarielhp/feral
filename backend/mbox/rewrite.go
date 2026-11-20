package mbox

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/email"
)

// Deleting from an mbox file is a rewrite of the file without the victims:
// one per call, however many messages it names. The file is changed only under
// the locks mail delivery honors (lock.go), only if it is still exactly the
// file the cache last read, and a victim is dropped only if its bytes still
// hash to the cached copy of that ID.
//
// Where a victim is found is positional, not derived: the folder index lists
// the file's messages in file order (the parser appends in order and a rewrite
// removes in place), so the k-th message that has an ID is ids[k]. Re-deriving
// IDs would not do -- the -N suffix of a duplicate Message-ID depends on what
// came before it, so a rewrite that drops the first copy would rename the
// second -- and the hash check catches any misalignment.
//
// A rewrite leaves a marker (rewriteMarker) between writing the new file and
// telling the cache, so a crash in between is settled by the next open instead
// of by a full re-parse, which would renumber exactly those duplicates.

// ErrMboxChanged reports a file that changed since the cache read it, such as
// new mail delivered since. Nothing is rewritten; reading the file again and
// retrying is safe.
var ErrMboxChanged = errors.New("the mbox file changed since it was read (new mail?); reload it and try again")

const rewriteMarkerFile = "mbox_rewrite.json"

// crashHook lets a test stop a rewrite at a named point, as a crash would:
// "before-rename" (the new file and the marker are written, the mbox is not
// replaced), "mid-compaction" (an in-place rewrite has written its first
// chunk) and "after-write" (the mbox is replaced, the cache not yet told).
var crashHook func(point string) error

func crashAt(point string) error {
	if crashHook == nil {
		return nil
	}
	return crashHook(point)
}

// rewriteMarker is a rewrite whose new file may be in place while the cache
// still describes the old one. It names both shapes of the file, so the next
// open can tell which one is there.
type rewriteMarker struct {
	Path    string     `json:"path"` // as configured; the cache's key
	OldSize int64      `json:"old_size"`
	OldTail string     `json:"old_tail"`
	NewSize int64      `json:"new_size"`
	NewTail string     `json:"new_tail"`
	Keep    []email.ID `json:"keep"`
	Drop    []email.ID `json:"drop"`
	// Tmp is the new file before it is renamed into place; a crash before
	// the rename leaves it, and settling removes it.
	Tmp string `json:"tmp,omitempty"`
	// InPlace marks a compaction of the file itself, which a crash can leave
	// neither old nor new while its size, head and tail look unchanged: so it
	// is settled by Compacted, set once the file is truncated and synced, and
	// by OldHash, the SHA-256 of the whole file before. Backup is that file.
	InPlace   bool   `json:"in_place,omitempty"`
	Compacted bool   `json:"compacted,omitempty"`
	OldHash   string `json:"old_hash,omitempty"`
	Backup    string `json:"backup,omitempty"`
}

// spoolDirs are where a system mailbox lives. The spool is the one mbox the
// user has no other copy of, so it is backed up before every rewrite.
var spoolDirs = []string{"/var/mail/", "/var/spool/mail/"}

func isSpool(path string) bool {
	for _, dir := range spoolDirs {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	return false
}

// rewritePlan is what a scan of the locked file decided.
type rewritePlan struct {
	keep    [][2]int64 // byte ranges to copy, in order
	keepIDs []email.ID
	dropped []email.ID
}

// rewrite is one rewrite of a locked mbox.
type rewrite struct {
	l      *lockedMbox
	acct   cachedir.Account
	fi     os.FileInfo // the file as locked
	marker rewriteMarker
	keep   [][2]int64
}

// deleteFromMbox rewrites the mbox at path without the messages victims names,
// and updates the cache in acct to match. An ID the cache no longer lists is
// already deleted and is skipped. path is the configured path, which keys the
// cache; a symlink is followed, so the file that changes is the real one and
// not the link.
func deleteFromMbox(path string, acct cachedir.Account, victims []email.ID) error {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve mbox %q: %w", path, err)
	}
	l, err := lockMbox(path, target)
	if err != nil {
		return err
	}
	defer l.release()
	if err = settleRewrite(l.file, path, acct); err != nil {
		return err
	}

	meta, err := loadMboxMeta(acct.Path())
	if err != nil {
		return fmt.Errorf("the mbox has not been read yet: %w", err)
	}
	fi, err := l.file.Stat()
	if err != nil {
		return err
	}
	if meta.Path != path || fi.Size() != meta.Size || !fi.ModTime().Equal(meta.ModTime) {
		return ErrMboxChanged
	}
	ids, err := inboxIDs(acct)
	if err != nil {
		return err
	}
	want, err := victimHashes(acct, listed(victims, ids))
	if err != nil || len(want) == 0 {
		return err
	}
	if _, err = l.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	plan, err := planRewrite(l.file, ids, want)
	if err != nil {
		return err
	}
	if isSpool(target) {
		if _, err = backupMbox(l.file, fi.Size(), filepath.Join(acct.Path(), "spool.bak")); err != nil {
			return fmt.Errorf("back up the spool before rewriting it: %w", err)
		}
	}
	rw := &rewrite{l: l, acct: acct, fi: fi, keep: plan.keep}
	return rw.run(path, meta, plan)
}

// inboxIDs is the folder index of the file; none yet is none.
func inboxIDs(acct cachedir.Account) ([]email.ID, error) {
	ids, err := folderindex.IDs(acct, "INBOX")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return ids, err
}

// listed is the victims the index still lists, in their given order.
func listed(victims, ids []email.ID) []email.ID {
	present := make(map[email.ID]bool, len(ids))
	for _, id := range ids {
		present[id] = true
	}
	var out []email.ID
	for _, id := range victims {
		if present[id] {
			out = append(out, id)
		}
	}
	return out
}

// victimHashes is the SHA-256 of each victim's cached copy, which its bytes in
// the file must still match.
func victimHashes(acct cachedir.Account, victims []email.ID) (map[email.ID][32]byte, error) {
	want := make(map[email.ID][32]byte, len(victims))
	for _, id := range victims {
		raw, err := msg.Read(acct, id)
		if err != nil {
			return nil, fmt.Errorf("message %s is not in the cache: %w", id, err)
		}
		want[id] = sha256.Sum256(raw)
	}
	return want, nil
}

// planRewrite scans the file and decides which spans to keep, matching each
// message that has an ID to ids by position.
func planRewrite(r io.Reader, ids []email.ID, want map[email.ID][32]byte) (rewritePlan, error) {
	var plan rewritePlan
	k := 0
	err := scanMbox(r, func(sp mboxSpan) error {
		if sp.preamble || len(bytes.TrimSpace(sp.content)) == 0 {
			plan.keep = append(plan.keep, [2]int64{sp.start, sp.end})
			return nil
		}
		if k >= len(ids) {
			return fmt.Errorf("the mbox holds more messages than its cache (%d); reload it and try again", len(ids))
		}
		id := ids[k]
		k++
		h, isVictim := want[id]
		if !isVictim {
			plan.keep = append(plan.keep, [2]int64{sp.start, sp.end})
			plan.keepIDs = append(plan.keepIDs, id)
			return nil
		}
		if sha256.Sum256(sp.content) != h {
			return fmt.Errorf("message %s in the file does not match its cached copy; nothing was deleted", id)
		}
		plan.dropped = append(plan.dropped, id)
		return nil
	})
	if err != nil {
		return rewritePlan{}, err
	}
	if k != len(ids) {
		return rewritePlan{}, fmt.Errorf("the mbox holds %d messages but its cache lists %d; reload it and try again", k, len(ids))
	}
	return plan, nil
}

// run puts the planned file in place and tells the cache: by a new file
// renamed over the old one when the directory allows it, otherwise by
// compacting the file in place (after a backup, since that is not atomic).
func (rw *rewrite) run(path string, meta *mboxMeta, plan rewritePlan) error {
	oldTail, err := computeTailHash(rw.l.file, rw.fi.Size())
	if err != nil {
		return err
	}
	newSize, newTail, err := plannedTail(rw.l.file, plan.keep)
	if err != nil {
		return err
	}
	rw.marker = rewriteMarker{
		Path: path, OldSize: rw.fi.Size(), OldTail: oldTail, NewSize: newSize, NewTail: newTail,
		Keep: plan.keepIDs, Drop: plan.dropped,
	}
	tmp, err := os.CreateTemp(filepath.Dir(rw.l.target), "."+filepath.Base(rw.l.target)+".feral-*")
	switch {
	case err == nil:
		err = rw.viaRename(tmp)
	case errors.Is(err, os.ErrPermission):
		err = rw.inPlace()
	}
	if err != nil {
		return err
	}
	if err := crashAt("after-write"); err != nil {
		return err
	}
	return applyRewrite(rw.acct, meta, rw.marker)
}

// viaRename writes the kept spans to tmp, syncs it, records the marker, and
// renames it over the mbox.
func (rw *rewrite) viaRename(tmp *os.File) error {
	tmpPath := tmp.Name()
	cleanup := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	for _, r := range rw.keep {
		if _, err := io.Copy(tmp, io.NewSectionReader(rw.l.file, r[0], r[1]-r[0])); err != nil {
			return cleanup(err)
		}
	}
	_ = tmp.Chmod(rw.fi.Mode().Perm())
	// A spool is user:mail; keep the group, which a new file would not have.
	if st, ok := rw.fi.Sys().(*syscall.Stat_t); ok {
		_ = tmp.Chown(-1, int(st.Gid))
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		return cleanup(err)
	}
	rw.marker.Tmp = tmpPath
	if err := writeMarker(rw.acct, rw.marker); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := crashAt("before-rename"); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, rw.l.target); err != nil {
		_ = os.Remove(tmpPath)
		_ = os.Remove(filepath.Join(rw.acct.Path(), rewriteMarkerFile))
		return err
	}
	syncDir(filepath.Dir(rw.l.target))
	return nil
}

// inPlace compacts the kept spans to the front of the locked file and
// truncates it. The kept spans are in order and never overlap what is yet to
// be read, so writing behind the read position is safe; a crash midway is
// not, which is why the file is backed up first and the marker says so.
func (rw *rewrite) inPlace() error {
	backup := filepath.Join(rw.acct.Path(), "mbox.bak")
	hash, err := backupMbox(rw.l.file, rw.fi.Size(), backup)
	if err != nil {
		return fmt.Errorf("back up the mbox before rewriting it in place: %w", err)
	}
	rw.marker.InPlace, rw.marker.Backup, rw.marker.OldHash = true, backup, hash
	if err := writeMarker(rw.acct, rw.marker); err != nil {
		return err
	}
	var w int64
	buf := make([]byte, 64*1024)
	for _, r := range rw.keep {
		for pos := r[0]; pos < r[1]; {
			n, err := rw.l.file.ReadAt(buf[:min(int64(len(buf)), r[1]-pos)], pos)
			if n == 0 {
				return fmt.Errorf("read mbox at %d: %w", pos, errors.Join(err, io.ErrUnexpectedEOF))
			}
			if _, werr := rw.l.file.WriteAt(buf[:n], w); werr != nil {
				return werr
			}
			w += int64(n)
			pos += int64(n)
			if cerr := crashAt("mid-compaction"); cerr != nil {
				return cerr
			}
		}
	}
	if err := rw.l.file.Truncate(w); err != nil {
		return err
	}
	if err := rw.l.file.Sync(); err != nil {
		return err
	}
	rw.marker.Compacted = true
	return writeMarker(rw.acct, rw.marker)
}

// plannedTail is the size and tail hash the rewritten file will have, read
// from the kept spans of the current one.
func plannedTail(f io.ReaderAt, keep [][2]int64) (int64, string, error) {
	var size int64
	for _, r := range keep {
		size += r[1] - r[0]
	}
	const sample = 4096
	var tail []byte
	for i := len(keep) - 1; i >= 0 && len(tail) < sample; i-- {
		r := keep[i]
		n := min(r[1]-r[0], int64(sample-len(tail)))
		chunk := make([]byte, n)
		if _, err := f.ReadAt(chunk, r[1]-n); err != nil && err != io.EOF {
			return 0, "", err
		}
		tail = append(chunk, tail...)
	}
	if size == 0 {
		return 0, "", nil
	}
	return size, fmt.Sprintf("%x", sha256.Sum256(tail)), nil
}

func writeMarker(acct cachedir.Account, marker rewriteMarker) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return dirlock.AtomicWrite(filepath.Join(acct.Path(), rewriteMarkerFile), data, 0o600)
}

// startsAs reports whether the file of the given size holds, at its start,
// the shape (size, tail hash) a marker recorded -- possibly with mail
// delivered after it.
func startsAs(f io.ReaderAt, size, shapeSize int64, shapeTail string) (bool, error) {
	if size < shapeSize {
		return false, nil
	}
	tail, err := computeTailHash(f, shapeSize)
	return tail == shapeTail, err
}

// settleRewrite finishes telling the cache about a rewrite an earlier run
// completed but crashed before recording, or forgets one that never happened.
// An in-place rewrite that was interrupted leaves a file that is neither, and
// that is an error naming the backup: reading it would take the damage for
// mail. f must be the locked mbox, or nil to open it; it never closes a
// descriptor of the mbox while the caller holds the fcntl lock.
func settleRewrite(f *os.File, path string, acct cachedir.Account) error {
	markerPath := filepath.Join(acct.Path(), rewriteMarkerFile)
	data, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var marker rewriteMarker
	if jsonErr := json.Unmarshal(data, &marker); jsonErr != nil || marker.Path != path {
		return os.Remove(markerPath)
	}
	if f == nil {
		opened, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer opened.Close()
		f = opened
	}
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if marker.InPlace {
		return settleInPlace(f, fi.Size(), acct, marker)
	}
	old, err := startsAs(f, fi.Size(), marker.OldSize, marker.OldTail)
	if err != nil {
		return err
	}
	if old {
		// The new file never went in.
		if marker.Tmp != "" {
			_ = os.Remove(marker.Tmp)
		}
		return os.Remove(markerPath)
	}
	done, err := startsAs(f, fi.Size(), marker.NewSize, marker.NewTail)
	if err != nil {
		return err
	}
	if meta, metaErr := loadMboxMeta(acct.Path()); done && metaErr == nil {
		return applyRewrite(acct, meta, marker)
	}
	// Something else replaced the file since; reading it afresh is right.
	return os.Remove(markerPath)
}

// settleInPlace settles an in-place compaction: finished, it is applied; not
// begun (the file is byte for byte what was backed up), it is forgotten;
// anything else is a damaged file, and an error that names the backup, so
// that neither a read nor another rewrite -- which would overwrite the backup
// -- touches it.
func settleInPlace(f *os.File, size int64, acct cachedir.Account, marker rewriteMarker) error {
	markerPath := filepath.Join(acct.Path(), rewriteMarkerFile)
	if marker.Compacted {
		meta, err := loadMboxMeta(acct.Path())
		if err != nil {
			return os.Remove(markerPath)
		}
		return applyRewrite(acct, meta, marker)
	}
	if size == marker.OldSize {
		h := sha256.New()
		if _, err := io.Copy(h, io.NewSectionReader(f, 0, size)); err != nil {
			return err
		}
		if fmt.Sprintf("%x", h.Sum(nil)) == marker.OldHash {
			return os.Remove(markerPath)
		}
	}
	return fmt.Errorf("an in-place rewrite of %s was interrupted and the file is damaged; "+
		"the file as it was before is in %s: restore it, then delete %s", marker.Path, marker.Backup, markerPath)
}

// applyRewrite makes the cache describe the rewritten file: the index keeps
// the survivors in order, the dropped messages leave the cache, and the
// metadata takes the new size and tail so the next open reads nothing -- or,
// if mail arrived since, reads only that, through the append path.
func applyRewrite(acct cachedir.Account, meta *mboxMeta, marker rewriteMarker) error {
	if err := folderindex.ReplaceAll(acct, "INBOX", marker.Keep); err != nil {
		return err
	}
	if _, err := msg.DeleteBatch(acct, marker.Drop); err != nil {
		return err
	}
	fi, err := os.Stat(marker.Path)
	if err != nil {
		return err
	}
	updated := mboxMeta{
		Path: marker.Path, Size: marker.NewSize, ModTime: fi.ModTime(), Count: len(marker.Keep),
		TailHash: marker.NewTail, LastCompactedSize: meta.LastCompactedSize,
	}
	data, err := json.Marshal(updated)
	if err != nil {
		return err
	}
	if err := dirlock.AtomicWrite(metaPath(acct.Path()), data, 0o600); err != nil {
		return err
	}
	return os.Remove(filepath.Join(acct.Path(), rewriteMarkerFile))
}

// backupMbox copies size bytes of f to dest, through f itself: opening a
// second descriptor of a locked mbox and closing it would drop the fcntl lock.
// It returns the SHA-256 of what it copied.
func backupMbox(f *os.File, size int64, dest string) (string, error) {
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), io.NewSectionReader(f, 0, size)); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), os.Rename(tmp, dest)
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
