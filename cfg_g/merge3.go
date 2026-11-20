package cfg_g

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// A save that finds a shared file changed since it was loaded -- another
// feral process saved in between -- merges instead of overwriting: base is
// what this run loaded, ours what it holds now, theirs what is on disk. A
// value this run did not change is taken from disk, so the other process's
// edit stands; a value this run changed is written. What the plan's later
// field policy (plans/config_sync.md §5) will refine, this does in its
// simplest form:
//
//   - an object merges key by key, at every depth, a key absent on one side
//     being a deletion when the base has it and an addition otherwise;
//   - a set (setKeys, setFiles) merges element by element: an element either
//     side added stays, one either side removed goes;
//   - accounts merge by name, so two runs that each add an account keep both;
//   - anything else -- a scalar, an ordered list such as the rules -- is one
//     value: when both sides changed it, differently, this run's change wins
//     and the conflict is logged. Before this merge the whole file was this
//     run's, so nothing is lost that was not lost before.

// absentValue stands for a key or file that is not there.
type absentValue struct{}

var absent any = absentValue{}

// mergeContext is where in which file a merge is.
type mergeContext struct {
	file string
	key  string
}

func (c mergeContext) at(key string) mergeContext {
	if c.key == "" {
		return mergeContext{file: c.file, key: key}
	}
	return mergeContext{file: c.file, key: c.key + "." + key}
}

// mergeValues is the three-way merge of one value.
func mergeValues(c mergeContext, base, ours, theirs any) any {
	switch {
	case reflect.DeepEqual(ours, base):
		return theirs
	case reflect.DeepEqual(theirs, base), reflect.DeepEqual(ours, theirs):
		return ours
	}
	if merged, ok := mergeStructured(c, base, ours, theirs); ok {
		return merged
	}
	slog.Warn("config: this run and another both changed a setting; keeping this run's",
		slog.String("file", c.file), slog.String("key", c.key))
	return ours
}

// mergeStructured merges what has structure to merge by: two objects, two
// sets, two account lists. ok is false for anything else.
func mergeStructured(c mergeContext, base, ours, theirs any) (any, bool) {
	if isSet(c) {
		if b, o, th, ok := stringLists(base, ours, theirs); ok {
			return mergeSets(b, o, th), true
		}
	}
	if c.key == "accounts" && filepath.Base(c.file) == "config.json" {
		if b, o, th, ok := objectLists(base, ours, theirs); ok {
			return mergeByName(c, b, o, th), true
		}
	}
	b, bok := asObject(base)
	o, ook := ours.(map[string]any)
	th, tok := theirs.(map[string]any)
	if !bok || !ook || !tok {
		return nil, false
	}
	return mergeObjects(c, b, o, th), true
}

// isSet reports whether the value at c is a set: a whole set file, or a
// set key at the top of config.json or account.json.
func isSet(c mergeContext) bool {
	if c.key == "" {
		return slices.Contains(setFiles, filepath.Base(c.file))
	}
	return slices.Contains(setKeys, c.key)
}

// asObject is v as an object, an absent base counting as an empty one: two
// runs that each add the same object merge its keys.
func asObject(v any) (map[string]any, bool) {
	if v == absent {
		return map[string]any{}, true
	}
	m, ok := v.(map[string]any)
	return m, ok
}

func mergeObjects(c mergeContext, base, ours, theirs map[string]any) map[string]any {
	keys := map[string]bool{}
	for _, m := range []map[string]any{base, ours, theirs} {
		for k := range m {
			keys[k] = true
		}
	}
	out := map[string]any{}
	for k := range keys {
		merged := mergeValues(c.at(k), lookup(base, k), lookup(ours, k), lookup(theirs, k))
		if merged != absent {
			out[k] = merged
		}
	}
	return out
}

func lookup(m map[string]any, key string) any {
	if v, ok := m[key]; ok {
		return v
	}
	return absent
}

// stringLists is the three values as lists of strings, an absent one empty.
func stringLists(vals ...any) (base, ours, theirs []string, ok bool) {
	lists := make([][]string, len(vals))
	for i, v := range vals {
		if v == absent {
			continue
		}
		raw, isList := v.([]any)
		if !isList {
			return nil, nil, nil, false
		}
		for _, e := range raw {
			s, isString := e.(string)
			if !isString {
				return nil, nil, nil, false
			}
			lists[i] = append(lists[i], s)
		}
	}
	return lists[0], lists[1], lists[2], true
}

// mergeSets keeps an element of the base only if neither side removed it,
// and an element not in the base if either side added it.
func mergeSets(base, ours, theirs []string) any {
	var out []string
	for _, e := range slices.Concat(ours, theirs) {
		if slices.Contains(out, e) {
			continue
		}
		inBase := slices.Contains(base, e)
		kept := slices.Contains(ours, e) && slices.Contains(theirs, e)
		if !inBase || kept {
			out = append(out, e)
		}
	}
	slices.Sort(out)
	list := make([]any, len(out))
	for i, e := range out {
		list[i] = e
	}
	return list
}

// objectLists is the three values as lists of objects, an absent one empty.
func objectLists(vals ...any) (base, ours, theirs []any, ok bool) {
	lists := make([][]any, len(vals))
	for i, v := range vals {
		if v == absent {
			continue
		}
		raw, isList := v.([]any)
		if !isList {
			return nil, nil, nil, false
		}
		for _, e := range raw {
			if _, isObject := e.(map[string]any); !isObject {
				return nil, nil, nil, false
			}
		}
		lists[i] = raw
	}
	return lists[0], lists[1], lists[2], true
}

// mergeByName merges config.json's accounts entry by entry, matched by
// name. The order is theirs, with the accounts this run added after it:
// both sides changed the list, and only an account's addition, not a
// reordering, is something a command does.
func mergeByName(c mergeContext, base, ours, theirs []any) any {
	b, o, th := byName(base), byName(ours), byName(theirs)
	var order []string
	for _, list := range [][]any{theirs, ours} {
		for _, e := range list {
			name := entryName(e)
			if !slices.Contains(order, name) {
				order = append(order, name)
			}
		}
	}
	out := []any{}
	for _, name := range order {
		merged := mergeValues(c.at(name), lookup(b, name), lookup(o, name), lookup(th, name))
		if merged != absent {
			out = append(out, merged)
		}
	}
	return out
}

func byName(list []any) map[string]any {
	m := map[string]any{}
	for _, e := range list {
		m[entryName(e)] = e
	}
	return m
}

func entryName(e any) string {
	obj, _ := e.(map[string]any)
	name, _ := obj["name"].(string)
	return name
}

// mergeSharedFile merges the three versions of one shared file: base and
// ours as this build renders them, theirs as read from disk (nil when the
// file is gone). A file that does not parse is refused rather than
// overwritten, as the loader refuses it.
func mergeSharedFile(path string, base, ours, theirs []byte) ([]byte, error) {
	values := make([]any, 3)
	for i, data := range [][]byte{base, ours, theirs} {
		if data == nil {
			values[i] = absent
			continue
		}
		v, err := decodeJSONValue(data)
		if err != nil {
			return nil, damagedFile(path, err)
		}
		values[i] = v
	}
	merged := mergeValues(mergeContext{file: path}, values[0], values[1], values[2])
	if merged == absent {
		merged = values[1]
	}
	return json.Marshal(merged)
}

// accountNames is the names in a config.json's accounts list, lower-cased.
func accountNames(configJSON []byte) map[string]bool {
	var top struct {
		Accounts []struct {
			Name string `json:"name"`
		} `json:"accounts"`
	}
	names := map[string]bool{}
	if json.Unmarshal(configJSON, &top) != nil {
		return names
	}
	for _, a := range top.Accounts {
		names[strings.ToLower(a.Name)] = true
	}
	return names
}
