// Package jsonx keeps the JSON fields a struct doesn't know when a record
// is read and written back. Stable and Nightly builds (and older and newer
// versions) share records — teams.json on a computer, team.json and member
// and project records in a team's storage — so a build that rewrites one
// must not drop what a newer build added.
//
// A record type embeds Extra and passes its JSON through Decode and Encode:
//
//	type Info struct {
//		Name  string      `json:"name"`
//		Extra jsonx.Extra `json:"-"`
//	}
//
//	func (i *Info) UnmarshalJSON(b []byte) error {
//		type plain Info
//		return jsonx.Decode(b, (*plain)(i), &i.Extra)
//	}
//
//	func (i Info) MarshalJSON() ([]byte, error) {
//		type plain Info
//		return jsonx.Encode(plain(i), i.Extra)
//	}
package jsonx

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// Extra holds the fields a record's type doesn't know, as they were.
type Extra map[string]json.RawMessage

// Decode decodes data into v (a pointer to a struct) and keeps the fields
// v's type doesn't know in extra.
func Decode(data []byte, v any, extra *Extra) error {
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	known := fields(reflect.TypeOf(v).Elem())
	*extra = nil
	for k, raw := range all {
		if !known[k] && !known[strings.ToLower(k)] {
			if *extra == nil {
				*extra = Extra{}
			}
			(*extra)[k] = raw
		}
	}
	return nil
}

// Encode encodes v (a struct) with extra's fields added; v's own fields win.
func Encode(v any, extra Extra) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil || len(extra) == 0 {
		return data, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		if _, ok := all[k]; !ok {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return data, nil
	}
	sort.Strings(keys)
	// v's fields in its own order, then the extra ones.
	var b bytes.Buffer
	b.Write(bytes.TrimSuffix(bytes.TrimSpace(data), []byte("}")))
	for _, k := range keys {
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		name, _ := json.Marshal(k)
		b.Write(name)
		b.WriteByte(':')
		b.Write(extra[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

var fieldCache sync.Map // reflect.Type -> map[string]bool

// fields: the JSON names a struct type decodes (embedded structs included).
func fields(t reflect.Type) map[string]bool {
	if m, ok := fieldCache.Load(t); ok {
		return m.(map[string]bool)
	}
	m := map[string]bool{}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			for k := range fields(f.Type) {
				m[k] = true
			}
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		m[name] = true
		// encoding/json matches names case-insensitively.
		m[strings.ToLower(name)] = true
	}
	fieldCache.Store(t, m)
	return m
}
