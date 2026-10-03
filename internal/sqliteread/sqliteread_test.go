package sqliteread

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures are made by Python's sqlite3 (see the commit adding them):
// plain.db has 600 rows on 1 KB pages (interior pages, overflow for long
// names); wal.db has 50 rows in the file and 70 more only in wal.db-wal.

func check(t *testing.T, rows []Row, n int) {
	t.Helper()
	if len(rows) != n {
		t.Fatalf("%d rows, want %d", len(rows), n)
	}
	for i, r := range rows {
		if r.Rowid != int64(i+1) || r.Values[0] != nil {
			t.Fatalf("row %d: rowid %d, first column %v", i, r.Rowid, r.Values[0])
		}
		name := fmt.Sprintf("Plugin %d", i)
		if i%97 == 0 {
			name += strings.Repeat("x", 3000)
		}
		var vendor any = "Vendor"
		if i%5 == 0 {
			vendor = nil
		}
		want := []any{fmt.Sprintf("device:vst3:instr:%08x-0000", i), name, vendor, fmt.Sprintf("1.%d", i), int64(-i),
			float64(i) / 4}
		for k, w := range want {
			got := r.Values[k+1]
			if k == 5 && i%4 == 0 { // SQLite keeps whole REAL values as integers
				if g, ok := got.(int64); ok {
					got = float64(g)
				}
			}
			if got != w {
				t.Fatalf("row %d column %d: %v, want %v", i, k+1, got, w)
			}
		}
		if b, ok := r.Values[7].([]byte); !ok || len(b) != 3 || b[0] != byte(i%256) {
			t.Fatalf("row %d blob %v", i, r.Values[7])
		}
	}
}

func TestPlain(t *testing.T) {
	db, err := Open("testdata/plain.db")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Table("plugins")
	if err != nil {
		t.Fatal(err)
	}
	check(t, rows, 600)
	if _, err := db.Table("nope"); err == nil {
		t.Error("no such table")
	}
}

func TestWAL(t *testing.T) {
	db, err := Open("testdata/wal.db")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Table("plugins")
	if err != nil {
		t.Fatal(err)
	}
	check(t, rows, 120)

	// Without its log: what the file alone has.
	dir := t.TempDir()
	data, _ := os.ReadFile("testdata/wal.db")
	os.WriteFile(filepath.Join(dir, "x.db"), data, 0o644)
	db, err = Open(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ = db.Table("plugins")
	check(t, rows, 50)
}

func TestNotSQLite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	os.WriteFile(p, []byte("hello"), 0o644)
	if _, err := Open(p); err != ErrNotSQLite {
		t.Fatal(err)
	}
}

func TestRecords(t *testing.T) {
	db, err := Open("testdata/plain.db")
	if err != nil {
		t.Fatal(err)
	}
	rs, err := db.Records("plugins")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 600 || rs[3]["plugin_id"] != int64(4) || rs[3]["name"] != "Plugin 3" || rs[3]["version"] != "1.3" {
		t.Fatalf("%v", rs[3])
	}
}
