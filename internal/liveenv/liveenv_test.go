package liveenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"12.1.5", "12.3", -1}, {"12.3", "12.3.0", 0}, {"12.10", "12.9", 1}, {"", "11", -1}, {"12b5", "12", 0}} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d", c.a, c.b, got)
		}
	}
}

// A computer laid out as Live does it.
func TestReadFrom(t *testing.T) {
	root := t.TempDir()
	d := Dirs{ProgramData: filepath.Join(root, "pd"), AppData: filepath.Join(root, "ad"), LocalAppData: filepath.Join(root, "lad")}
	prog := filepath.Join(d.ProgramData, "Ableton", "Live 12 Suite", "Program")
	os.MkdirAll(prog, 0o755)
	os.WriteFile(filepath.Join(prog, "Ableton Live 12 Suite.exe"), []byte("not really"), 0o644)
	os.WriteFile(filepath.Join(prog, "Installation.cfg"), []byte(`{"variant": "Suite"}`), 0o644)
	os.MkdirAll(filepath.Join(d.ProgramData, "Ableton", "Live 12 Suite", "Resources", "Core Library"), 0o755)
	prefs := filepath.Join(d.AppData, "Ableton", "Live 12.3.2", "Preferences")
	os.MkdirAll(prefs, 0o755)
	user := filepath.Join(root, "docs", "Ableton")
	os.MkdirAll(filepath.Join(user, "Factory Packs", "Drum Essentials"), 0o755)
	os.WriteFile(filepath.Join(prefs, "Library.cfg"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Ableton><ContentLibrary><UserLibrary><LibraryProject Id="0"><ProjectPath Value="`+filepath.ToSlash(user)+`" /></LibraryProject></UserLibrary>
<PreferredFactoryPacksInstallationPath Value="" /></ContentLibrary></Ableton>`), 0o644)
	db := filepath.Join(d.LocalAppData, "Ableton", "Live Database")
	os.MkdirAll(db, 0o755)
	data, _ := os.ReadFile(filepath.Join("testdata", "Live-plugins-1.db"))
	os.WriteFile(filepath.Join(db, "Live-plugins-1.db"), data, 0o644)

	e := ReadFrom(d, nil)
	if len(e.Installs) != 1 || e.Installs[0].Edition != "Suite" || e.Installs[0].Version == "" {
		t.Fatalf("installs %+v", e.Installs)
	}
	if !e.PacksKnown || len(e.Packs) != 2 || e.Packs[0] != "Core Library" || e.Packs[1] != "Drum Essentials" {
		t.Fatalf("packs %v", e.Packs)
	}
	if !e.PluginsKnown || len(e.Plugins) != 2 {
		t.Fatalf("plugins %+v", e.Plugins)
	}
	v := e.Plugins[0]
	if v.Name != "Vital" || v.Format != "VST3" || v.Version != "1.0.7" || v.UID() != "56535456-6974-6176-6974-616c00000000" ||
		v.File != `C:\Program Files\Common Files\VST3\Vital.vst3` {
		t.Fatalf("vital %+v", v)
	}
	if e.Plugins[1].Format != "VST" || e.Plugins[1].UID() != "1483109208" {
		t.Fatalf("vst2 %+v", e.Plugins[1])
	}

	// Nothing there: nothing known, no guesses.
	e = ReadFrom(Dirs{}, nil)
	if len(e.Installs) != 0 || e.PluginsKnown || e.PacksKnown {
		t.Fatalf("empty: %+v", e)
	}
}
