package cfg_g

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
)

// The shared files are written in one canonical layout, whatever produced
// them -- a struct marshalled by SaveConfigFile, keepUnknownKeys appending
// what this build does not know, or an account rename editing the raw JSON
// -- so that the same content is always the same bytes. Two machines that
// write the same configuration then write identical files, and a diff of
// one shows only what changed.
//
// The layout: every object's keys sorted, at every depth; two-space indent,
// one array element per line; a final newline. The arrays that are sets are
// sorted too (setKeys); an ordered list -- rules, editor_args, a link's
// args, save_locations, accounts -- keeps its order, which means something.

// setKeys are the keys, in config.json and account.json, whose arrays are
// sets: their order means nothing, so it is sorted away.
var setKeys = []string{"my_emails", "allowed_languages", "whitelist", "blacklist"}

// setFiles are the shared files that are themselves one set.
var setFiles = []string{"whitelist.json", "blacklist.json"}

// canonicalJSON is data, a JSON document destined for path, in the canonical
// layout. Numbers keep their text: a value this build does not know is
// carried byte for byte, not rewritten through float64.
func canonicalJSON(path string, data []byte) ([]byte, error) {
	v, err := decodeJSONValue(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if slices.Contains(setFiles, filepath.Base(path)) {
		v = sortedSet(v)
	}
	if obj, ok := v.(map[string]any); ok {
		for _, key := range setKeys {
			if set, present := obj[key]; present {
				obj[key] = sortedSet(set)
			}
		}
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return append(out, '\n'), nil
}

// decodeJSONValue decodes one JSON document into maps, slices and
// json.Number, refusing trailing data.
func decodeJSONValue(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("data after the JSON document")
	}
	return v, nil
}

// sortedSet is v sorted when it is an array of strings, and v otherwise.
func sortedSet(v any) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	strs := make([]string, 0, len(list))
	for _, e := range list {
		s, isString := e.(string)
		if !isString {
			return v
		}
		strs = append(strs, s)
	}
	slices.Sort(strs)
	out := make([]any, len(strs))
	for i, s := range strs {
		out[i] = s
	}
	return out
}
