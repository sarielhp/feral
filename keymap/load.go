package keymap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/sarielhp/feral/fileformat"
)

// A keys file is one object: "format", then a member per group, each an
// object from action names to a key or a list of keys:
//
//	{"format": 1, "list": {"archive": "A", "browser": ["w", "W"]}}
//
// An action left out keeps its default keys; one that is named runs on the
// keys given and on no other.

// Load reads the keys file in configDir. A missing file is the defaults.
func Load(t *Table, configDir string) (*Keymap, error) {
	path := filepath.Join(configDir, FileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(t), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(t, path, data)
}

// entry is one action a keys file binds.
type entry struct {
	group, action string
	keys          []string
	line          int
}

// lineError is a refusal at a line of the keys file.
type lineError struct {
	line  int
	where string
	err   error
}

func (e *lineError) Error() string {
	if e.where == "" {
		return fmt.Sprintf("line %d: %v", e.line, e.err)
	}
	return fmt.Sprintf("line %d: %s: %v", e.line, e.where, e.err)
}

// Parse is the keymap a keys file at path describes, or the refusal of its
// first entry that is wrong.
func Parse(t *Table, path string, data []byte) (*Keymap, error) {
	// The format first, wherever it stands in the object: a newer build's
	// file may name groups and actions this build lacks, and those are a
	// newer format, not mistakes.
	if err := fileformat.Check(path, probeFormat(data)); err != nil {
		return nil, err
	}
	r := &reader{dec: json.NewDecoder(bytes.NewReader(data)), data: data, t: t}
	r.dec.UseNumber()
	entries, err := r.file()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	km := Default(t)
	for _, e := range entries {
		keys, err := normalizeAll(e.keys)
		if err == nil {
			err = t.checkKeys(keys)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, &lineError{e.line, e.group + "." + e.action, err})
		}
		km.keys[e.group][e.action] = keys
	}
	if err := km.checkShared(t, entries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	km.index()
	return km, nil
}

// probeFormat is the file's format field, or 0 when it has none or does not
// parse; the reader judges the rest.
func probeFormat(data []byte) int {
	var probe struct {
		Format json.RawMessage `json:"format"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return 0
	}
	var format int
	if json.Unmarshal(probe.Format, &format) != nil {
		return 0
	}
	return format
}

func normalizeAll(keys []string) ([]string, error) {
	out := make([]string, len(keys))
	for i, k := range keys {
		n, err := Normalize(k)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// checkShared refuses a key two actions of one group would share once the
// file's entries replace their defaults. It blames the later of the entries
// that made the clash: the one a person most likely just added.
func (km *Keymap) checkShared(t *Table, entries []entry) error {
	line := map[string]int{}
	for _, e := range entries {
		line[e.group+"."+e.action] = e.line
	}
	for _, g := range t.Groups {
		owner := map[string]string{}
		for _, b := range g.Bindings {
			for _, k := range km.keys[g.Name][b.Action] {
				other, taken := owner[k]
				if !taken {
					owner[k] = b.Action
					continue
				}
				blame := b.Action
				if line[g.Name+"."+other] > line[g.Name+"."+blame] {
					blame, other = other, blame
				}
				return &lineError{line[g.Name+"."+blame], g.Name + "." + blame,
					fmt.Errorf("%q is also the key of %s; rebind %s too", Display(k), other, other)}
			}
		}
	}
	return nil
}

// reader walks a keys file token by token, so that every refusal can name
// its line: encoding/json's own errors name a byte offset at best, and it
// keeps the last of two members with one name without a word.
type reader struct {
	dec  *json.Decoder
	data []byte
	t    *Table
}

// line is the line the decoder has read up to.
func (r *reader) line() int {
	return 1 + bytes.Count(r.data[:r.dec.InputOffset()], []byte("\n"))
}

func (r *reader) fail(where string, err error) error {
	return &lineError{r.line(), where, err}
}

// file reads the whole object: each group's entries, and a format that must
// be a whole number (Parse has checked its value).
func (r *reader) file() ([]entry, error) {
	if err := r.open("", "the file must be a JSON object"); err != nil {
		return nil, err
	}
	var entries []entry
	seen := map[string]bool{}
	for r.dec.More() {
		name, err := r.member(seen, "")
		if err != nil {
			return nil, err
		}
		if name == "format" {
			if err = r.format(); err != nil {
				return nil, err
			}
			continue
		}
		group, err := r.group(name)
		if err != nil {
			return nil, err
		}
		entries = append(entries, group...)
	}
	if err := r.close(); err != nil {
		return nil, err
	}
	if _, err := r.dec.Token(); !errors.Is(err, io.EOF) {
		return nil, r.fail("", fmt.Errorf("unexpected text after the object"))
	}
	return entries, nil
}

// open reads the opening brace of an object.
func (r *reader) open(where, want string) error {
	tok, err := r.dec.Token()
	if err != nil {
		return r.fail(where, fmt.Errorf("not valid JSON: %w", err))
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return r.fail(where, errors.New(want))
	}
	return nil
}

// close reads the closing brace of an object whose members are read.
func (r *reader) close() error {
	if _, err := r.dec.Token(); err != nil {
		return r.fail("", fmt.Errorf("not valid JSON: %w", err))
	}
	return nil
}

// member reads a member's name and refuses one already seen in its object:
// encoding/json would keep the second without a word.
func (r *reader) member(seen map[string]bool, prefix string) (string, error) {
	tok, err := r.dec.Token()
	if err != nil {
		return "", r.fail(prefix, fmt.Errorf("not valid JSON: %w", err))
	}
	name, _ := tok.(string)
	if seen[name] {
		return "", r.fail(prefix+name, errors.New("named twice"))
	}
	seen[name] = true
	return name, nil
}

func (r *reader) format() error {
	tok, err := r.dec.Token()
	if err != nil {
		return r.fail("format", fmt.Errorf("not valid JSON: %w", err))
	}
	n, ok := tok.(json.Number)
	if !ok {
		return r.fail("format", errors.New("must be a number"))
	}
	if _, err := n.Int64(); err != nil {
		return r.fail("format", errors.New("must be a whole number"))
	}
	return nil
}

// group reads one group's object of actions.
func (r *reader) group(name string) ([]entry, error) {
	if !slices.ContainsFunc(r.t.Groups, func(g Group) bool { return g.Name == name }) {
		return nil, r.fail(name, fmt.Errorf("no such group (the groups are %s)", r.groupNames()))
	}
	if err := r.open(name, "must be an object of actions"); err != nil {
		return nil, err
	}
	var out []entry
	seen := map[string]bool{}
	for r.dec.More() {
		action, err := r.member(seen, name+".")
		if err != nil {
			return nil, err
		}
		where, line := name+"."+action, r.line()
		if !r.t.has(name, action) {
			return nil, r.fail(where, errors.New("no such action (feral utils keys lists them)"))
		}
		keys, err := r.keys(where)
		if err != nil {
			return nil, err
		}
		out = append(out, entry{group: name, action: action, keys: keys, line: line})
	}
	return out, r.close()
}

func (r *reader) groupNames() string {
	var names []string
	for _, g := range r.t.Groups {
		names = append(names, g.Name)
	}
	return fmt.Sprintf("%q", names)
}

// keys reads a key or a list of keys.
func (r *reader) keys(where string) ([]string, error) {
	tok, err := r.dec.Token()
	if err != nil {
		return nil, r.fail(where, fmt.Errorf("not valid JSON: %w", err))
	}
	if s, ok := tok.(string); ok {
		return []string{s}, nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, r.fail(where, errors.New("must be a key or a list of keys"))
	}
	var keys []string
	for r.dec.More() {
		tok, err := r.dec.Token()
		if err != nil {
			return nil, r.fail(where, fmt.Errorf("not valid JSON: %w", err))
		}
		s, ok := tok.(string)
		if !ok {
			return nil, r.fail(where, errors.New("must be a key or a list of keys"))
		}
		keys = append(keys, s)
	}
	return keys, r.close()
}
