package cfg_g

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
)

// keepUnknownKeys returns data -- a JSON object marshalled from a value of
// type t -- with every top-level key of the file at path that t does not know
// appended, its value unchanged.
//
// config.json and account.json are saved by marshalling a struct, so a save
// used to drop every key the running build did not declare: a setting written
// by a newer build, or added by hand, vanished on the next unrelated save
// (`account default`, the load-time auto-fix). A key t knows is never carried
// over, so clearing an omitempty field still removes it. A missing or
// unparsable file has nothing to keep; the loader has already refused a
// damaged one.
func keepUnknownKeys(path string, data []byte, t reflect.Type) []byte {
	old, err := os.ReadFile(path)
	if err != nil {
		return data
	}
	return keepUnknownKeysFrom(old, data, t)
}

// keepUnknownKeysFrom is keepUnknownKeys with the old file's bytes in hand.
func keepUnknownKeysFrom(old, data []byte, t reflect.Type) []byte {
	var fields map[string]json.RawMessage
	if json.Unmarshal(old, &fields) != nil || len(fields) == 0 {
		return data
	}
	known := knownJSONKeys(t)
	var unknown []string
	for key := range fields {
		if !known[strings.ToLower(key)] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return data
	}
	slices.Sort(unknown)
	return appendJSONKeys(data, unknown, fields)
}

// appendJSONKeys adds keys, with their values from fields, at the end of the
// two-space-indented JSON object data, in the same layout.
func appendJSONKeys(data []byte, keys []string, fields map[string]json.RawMessage) []byte {
	body := bytes.TrimRight(data, " \t\r\n")
	body = bytes.TrimSuffix(body, []byte("}"))
	body = bytes.TrimRight(body, " \t\r\n")
	empty := bytes.Equal(bytes.TrimSpace(body), []byte("{"))
	out := bytes.NewBuffer(append([]byte{}, body...))
	for i, key := range keys {
		if i > 0 || !empty {
			out.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		out.WriteString("\n  ")
		out.Write(name)
		out.WriteString(": ")
		if json.Indent(out, fields[key], "  ", "  ") != nil {
			out.Write(fields[key])
		}
	}
	out.WriteString("\n}")
	return out.Bytes()
}

// knownJSONKeys is the set of keys encoding/json decodes into t's fields,
// lower-cased because Unmarshal matches keys without regard to case.
func knownJSONKeys(t reflect.Type) map[string]bool {
	known := make(map[string]bool)
	addJSONKeys(known, t)
	return known
}

func addJSONKeys(known map[string]bool, t reflect.Type) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			addJSONKeys(known, f.Type)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		known[strings.ToLower(name)] = true
	}
}
