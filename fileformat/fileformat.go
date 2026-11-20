// Package fileformat is the version check every file the program writes goes
// through, so a build never reads -- and then overwrites -- a file in a format
// a newer build wrote.
//
// Two kinds of file need it. An object-shaped file (a saved result set, the
// watcher's state, config.json) carries a "format" field, absent meaning 1;
// its reader calls Check. A bare file (a training ledger, a white or black
// list, the read state) is an array, or a map whose values are booleans or
// objects, and keeps that shape: wrapping it would make a build from before
// this check read it as damage, and several such readers used to read damage
// as empty and save the empty file back. A later build that needs to change
// such a file wraps it as {"format": N, ...}; RefuseWrapped is how this build
// recognizes that and refuses it instead of misreading it. No bare file can
// look like that today: none of them has a "format" key whose value is a
// number.
package fileformat

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Current is the format every file is written in today. A file with no
// format field is format 1.
const Current = 1

// TooNewError refuses a file a newer build wrote.
type TooNewError struct {
	Path   string
	Format int
	// Readable is the newest format this build reads for the file; zero
	// means Current.
	Readable int
}

func (e *TooNewError) Error() string {
	readable := e.Readable
	if readable == 0 {
		readable = Current
	}
	return fmt.Sprintf("%s is in format %d, written by a newer feral; this build reads format %d. Install a newer feral",
		e.Path, e.Format, readable)
}

// Check refuses an object-shaped file whose format field is newer than this
// build reads. A zero format -- the field absent -- is format 1.
func Check(path string, format int) error {
	return CheckAt(path, format, Current)
}

// CheckAt is Check for a file whose own format has moved past Current:
// readable is the newest format this build reads for it. A file raises its
// own format, rather than Current, when only its shape changed, so that every
// other file stays readable by the builds before it.
func CheckAt(path string, format, readable int) error {
	if format > readable {
		return &TooNewError{Path: path, Format: format, Readable: readable}
	}
	return nil
}

// RefuseWrapped refuses a bare file that a newer build wrapped as
// {"format": N, ...}. Anything else -- a bare array or map, or bytes that do
// not parse at all -- passes, for the caller's own parser to judge.
func RefuseWrapped(path string, data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(trimmed, &probe) != nil {
		return nil
	}
	raw, ok := probe["format"]
	if !ok {
		return nil
	}
	var format int
	if json.Unmarshal(raw, &format) != nil {
		return nil // a map entry keyed "format" whose value is not a number
	}
	return &TooNewError{Path: path, Format: max(format, Current+1)}
}
