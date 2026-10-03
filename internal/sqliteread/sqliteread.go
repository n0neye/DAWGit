// Package sqliteread reads the rows of tables in an SQLite database file,
// without SQLite: enough to look into small databases other programs keep
// (Ableton Live's plugin list). Read only; pages a write-ahead log (-wal)
// holds, committed but not yet in the file, are read from it.
package sqliteread

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

// DB is an SQLite database read into memory.
type DB struct {
	data     []byte
	wal      map[uint32][]byte // page number: its newest committed copy in the WAL
	pageSize int
	usable   int
}

var ErrNotSQLite = errors.New("not an SQLite database")

// Open reads the database at path (and path-wal when there is one).
func Open(path string) (*DB, error) {
	data, err := readShared(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 100 || string(data[:16]) != "SQLite format 3\x00" {
		return nil, ErrNotSQLite
	}
	db := &DB{data: data, pageSize: int(binary.BigEndian.Uint16(data[16:18]))}
	if db.pageSize == 1 {
		db.pageSize = 65536
	}
	db.usable = db.pageSize - int(data[20])
	if wal, err := readShared(path + "-wal"); err == nil {
		db.wal = readWAL(wal, db.pageSize)
	}
	return db, nil
}

// readShared reads a file another program may have open for writing.
func readShared(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// readWAL maps each page to its copy in the last committed transaction
// that has it. Frames after the last commit, or left from an earlier log
// (other salts), don't count.
func readWAL(w []byte, pageSize int) map[uint32][]byte {
	if len(w) < 32 {
		return nil
	}
	if m := binary.BigEndian.Uint32(w[0:4]); m != 0x377f0682 && m != 0x377f0683 {
		return nil
	}
	if int(binary.BigEndian.Uint32(w[8:12])) != pageSize {
		return nil
	}
	salt1, salt2 := binary.BigEndian.Uint32(w[16:20]), binary.BigEndian.Uint32(w[20:24])
	out := map[uint32][]byte{}
	pending := map[uint32][]byte{}
	for off := 32; off+24+pageSize <= len(w); off += 24 + pageSize {
		h := w[off : off+24]
		if binary.BigEndian.Uint32(h[8:12]) != salt1 || binary.BigEndian.Uint32(h[12:16]) != salt2 {
			break
		}
		pending[binary.BigEndian.Uint32(h[0:4])] = w[off+24 : off+24+pageSize]
		if binary.BigEndian.Uint32(h[4:8]) != 0 { // a commit
			for p, d := range pending {
				out[p] = d
			}
			pending = map[uint32][]byte{}
		}
	}
	return out
}

func (db *DB) page(n uint32) ([]byte, error) {
	if p, ok := db.wal[n]; ok {
		return p, nil
	}
	start := int(n-1) * db.pageSize
	if n == 0 || start+db.pageSize > len(db.data) {
		return nil, fmt.Errorf("sqlite: page %d out of range", n)
	}
	return db.data[start : start+db.pageSize], nil
}

// Row is a table row: its rowid and column values (nil, int64, float64,
// string or []byte). An INTEGER PRIMARY KEY column reads as nil: its value
// is the rowid.
type Row struct {
	Rowid  int64
	Values []any
}

// Table reads every row of the table name.
func (db *DB) Table(name string) ([]Row, error) {
	master, err := db.rows(1, 0)
	if err != nil {
		return nil, err
	}
	for _, r := range master {
		if len(r.Values) >= 4 && r.Values[0] == "table" && strings.EqualFold(fmt.Sprint(r.Values[1]), name) {
			root, ok := r.Values[3].(int64)
			if !ok {
				return nil, fmt.Errorf("sqlite: table %s has no root page", name)
			}
			return db.rows(uint32(root), 0)
		}
	}
	return nil, fmt.Errorf("sqlite: no table %s", name)
}

// rows walks the table b-tree from page n.
func (db *DB) rows(n uint32, depth int) ([]Row, error) {
	if depth > 64 {
		return nil, errors.New("sqlite: b-tree too deep")
	}
	p, err := db.page(n)
	if err != nil {
		return nil, err
	}
	hdr := 0
	if n == 1 {
		hdr = 100
	}
	if hdr+8 > len(p) {
		return nil, errors.New("sqlite: short page")
	}
	kind := p[hdr]
	cells := int(binary.BigEndian.Uint16(p[hdr+3 : hdr+5]))
	ptrs := hdr + 8
	if kind == 0x05 {
		ptrs = hdr + 12
	}
	cell := func(i int) (int, error) {
		at := ptrs + 2*i
		if at+2 > len(p) {
			return 0, errors.New("sqlite: bad cell pointer")
		}
		c := int(binary.BigEndian.Uint16(p[at : at+2]))
		if c >= len(p) {
			return 0, errors.New("sqlite: bad cell pointer")
		}
		return c, nil
	}
	var out []Row
	switch kind {
	case 0x05: // interior: children, then the right-most one
		for i := 0; i < cells; i++ {
			c, err := cell(i)
			if err != nil || c+4 > len(p) {
				return nil, errors.New("sqlite: bad interior cell")
			}
			sub, err := db.rows(binary.BigEndian.Uint32(p[c:c+4]), depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
		sub, err := db.rows(binary.BigEndian.Uint32(p[hdr+8:hdr+12]), depth+1)
		if err != nil {
			return nil, err
		}
		return append(out, sub...), nil
	case 0x0d: // leaf
		for i := 0; i < cells; i++ {
			c, err := cell(i)
			if err != nil {
				return nil, err
			}
			size, k := varint(p[c:])
			rowid, k2 := varint(p[c+k:])
			payload, err := db.payload(p, c+k+k2, int(size))
			if err != nil {
				return nil, err
			}
			vals, err := record(payload)
			if err != nil {
				return nil, err
			}
			out = append(out, Row{Rowid: int64(rowid), Values: vals})
		}
		return out, nil
	}
	return nil, fmt.Errorf("sqlite: page %d is not a table page (%#x)", n, kind)
}

// payload reads a leaf cell's payload of size bytes starting at p[at:],
// following overflow pages.
func (db *DB) payload(p []byte, at, size int) ([]byte, error) {
	u := db.usable
	x := u - 35
	local := size
	if size > x {
		m := ((u-12)*32)/255 - 23
		k := m + (size-m)%(u-4)
		local = m
		if k <= x {
			local = k
		}
	}
	if at+local > len(p) {
		return nil, errors.New("sqlite: short cell")
	}
	out := append([]byte(nil), p[at:at+local]...)
	if local == size {
		return out, nil
	}
	if at+local+4 > len(p) {
		return nil, errors.New("sqlite: short cell")
	}
	next := binary.BigEndian.Uint32(p[at+local : at+local+4])
	for len(out) < size && next != 0 {
		op, err := db.page(next)
		if err != nil {
			return nil, err
		}
		n := min(size-len(out), u-4)
		out = append(out, op[4:4+n]...)
		next = binary.BigEndian.Uint32(op[0:4])
	}
	if len(out) != size {
		return nil, errors.New("sqlite: overflow chain too short")
	}
	return out, nil
}

// record decodes a record: a header of serial types, then the values.
func record(b []byte) ([]any, error) {
	hsize, k := varint(b)
	if int(hsize) > len(b) || k == 0 {
		return nil, errors.New("sqlite: bad record")
	}
	var types []uint64
	for at := k; at < int(hsize); {
		t, n := varint(b[at:])
		if n == 0 {
			return nil, errors.New("sqlite: bad record header")
		}
		types = append(types, t)
		at += n
	}
	body := b[hsize:]
	var out []any
	for _, t := range types {
		var n int
		switch {
		case t == 0, t == 8, t == 9:
			n = 0
		case t >= 1 && t <= 4:
			n = int(t)
		case t == 5:
			n = 6
		case t == 6, t == 7:
			n = 8
		case t >= 12:
			n = int(t-12) / 2
		default:
			return nil, fmt.Errorf("sqlite: serial type %d", t)
		}
		if n > len(body) {
			return nil, errors.New("sqlite: record too short")
		}
		v := body[:n]
		body = body[n:]
		switch {
		case t == 0:
			out = append(out, nil)
		case t == 8:
			out = append(out, int64(0))
		case t == 9:
			out = append(out, int64(1))
		case t <= 6:
			var x int64
			for _, c := range v {
				x = x<<8 | int64(c)
			}
			shift := 64 - 8*uint(n) // sign-extend
			out = append(out, x<<shift>>shift)
		case t == 7:
			out = append(out, math.Float64frombits(binary.BigEndian.Uint64(v)))
		case t%2 == 0:
			out = append(out, append([]byte(nil), v...))
		default:
			out = append(out, string(v))
		}
	}
	return out, nil
}

// varint reads SQLite's big-endian varint: the value and its length.
func varint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < 9 && i < len(b); i++ {
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, 0
}

// Records reads the table name as rows keyed by column name; an INTEGER
// PRIMARY KEY column has the rowid.
func (db *DB) Records(name string) ([]map[string]any, error) {
	master, err := db.rows(1, 0)
	if err != nil {
		return nil, err
	}
	var cols []string
	var key string
	for _, r := range master {
		if len(r.Values) >= 5 && r.Values[0] == "table" && strings.EqualFold(fmt.Sprint(r.Values[1]), name) {
			sql, _ := r.Values[4].(string)
			cols, key = columns(sql)
		}
	}
	if cols == nil {
		return nil, fmt.Errorf("sqlite: no table %s", name)
	}
	rows, err := db.Table(name)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m := map[string]any{}
		for i, c := range cols {
			if i < len(r.Values) {
				m[c] = r.Values[i]
			}
		}
		if key != "" {
			m[key] = r.Rowid
		}
		out = append(out, m)
	}
	return out, nil
}

// columns reads the column names of a CREATE TABLE statement, and the one
// that is the rowid (INTEGER PRIMARY KEY), if any.
func columns(sql string) (cols []string, rowid string) {
	open, close := strings.Index(sql, "("), strings.LastIndex(sql, ")")
	if open < 0 || close < open {
		return nil, ""
	}
	depth, start := 0, open+1
	var parts []string
	for i := open + 1; i < close; i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, sql[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, sql[start:close])
	for _, p := range parts {
		f := strings.Fields(p)
		if len(f) == 0 {
			continue
		}
		switch strings.ToUpper(f[0]) {
		case "PRIMARY", "UNIQUE", "CHECK", "FOREIGN", "CONSTRAINT":
			continue
		}
		name := strings.Trim(f[0], "\"`[]")
		cols = append(cols, name)
		up := strings.ToUpper(strings.Join(f[1:], " "))
		if strings.HasPrefix(up, "INTEGER") && strings.Contains(up, "PRIMARY KEY") {
			rowid = name
		}
	}
	return cols, rowid
}
