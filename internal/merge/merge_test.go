package merge

import (
	"path/filepath"
	"sort"
	"testing"

	"dawgit/internal/als"
	"dawgit/internal/diff"
)

// Native tests on real fixtures; they do not need the Python golden data.

var project = filepath.Join("..", "..", "SampleProjects", "SampleAbletonProject Project")

func fixture(t *testing.T, name string) *als.LiveSet {
	t.Helper()
	return load(t, filepath.Join(project, name))
}

func byName(s *als.LiveSet) map[string]als.Track {
	m := map[string]als.Track{}
	for _, t := range s.Tracks() {
		m[t.Name()] = t
	}
	return m
}

func mustMerge(t *testing.T, base, ours, theirs *als.LiveSet, strategy string) *Result {
	t.Helper()
	r, err := Merge(base, ours, theirs, strategy)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Issues) > 0 {
		t.Fatalf("validation issues:\n%s", r.Report())
	}
	// Survives a serialize/parse cycle.
	if _, err := als.FromXML(r.Merged.XML()); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFixturesAreValid(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(project, "*.als"))
	backups, _ := filepath.Glob(filepath.Join(project, "Backup", "*.als"))
	for _, f := range append(files, backups...) {
		if filepath.Base(f)[:min(10, len(filepath.Base(f)))] == "MergeTest-" {
			continue
		}
		if issues := als.Validate(load(t, f)); len(issues) > 0 {
			t.Errorf("%s: %v", filepath.Base(f), issues)
		}
	}
}

func TestIdenticalSidesIsNoop(t *testing.T) {
	base := fixture(t, "SampleAbletonProject_v2.als")
	r := mustMerge(t, base, base.Clone(), base.Clone(), "fail")
	if len(r.Conflicts) > 0 || !diff.Diff(base, r.Merged).Empty() {
		t.Fatalf("expected no-op, got:\n%s", r.Report())
	}
}

func TestSaveOnlyNoiseHasNoDiff(t *testing.T) {
	a := fixture(t, filepath.Join("Backup", "SampleAbletonProject [2026-09-28 200235].als"))
	b := fixture(t, filepath.Join("Backup", "SampleAbletonProject [2026-09-28 200308].als"))
	if d := diff.Diff(a, b); !d.Empty() {
		t.Fatalf("expected no changes:\n%s", d.Render())
	}
}

// Split-A and Split-B were both edited by hand in Live from the same v2.
func TestRealSplit(t *testing.T) {
	base := fixture(t, "SampleAbletonProject_v2.als")
	a, b := fixture(t, "Split-A.als"), fixture(t, "Split-B.als")

	r := mustMerge(t, base, a, b, "fail")
	if len(r.Conflicts) != 1 || r.Conflicts[0].Unit != `AudioTrack "5 Bounce + Reverb"` {
		t.Fatalf("conflicts: %+v", r.Conflicts)
	}

	m := mustMerge(t, base, a, b, "both").Merged
	tracks := byName(m)
	audios, drum := tracks["Audios"], tracks["Drum"]
	if audios.Kind() != "GroupTrack" || drum.ID() == audios.ID() {
		t.Fatalf("group %v / drum %v", audios.ID(), drum.ID())
	}
	beat := tracks["4-80s Beat 90 bpm"]
	if beat.GroupID() != audios.ID() {
		t.Errorf("beat group = %s, want %s", beat.GroupID(), audios.ID())
	}
	if on := beat.Elem.Val("DeviceChain/Mixer/Speaker/Manual", ""); on != "false" {
		t.Errorf("beat track on = %s (theirs muted it)", on)
	}
	cp := tracks["# Bounce + Reverb [theirs]"]
	if cp.GroupID() != audios.ID() {
		t.Errorf("conflict copy not in ours' group")
	}
	var auto []string
	for k := range diff.Envelopes(tracks["6 Bounce + Reverb"].Elem) {
		auto = append(auto, k)
	}
	sort.Strings(auto)
	if len(auto) != 2 || auto[0] != "Mixer: Volume" || auto[1] != "Reverb: MixDirect" {
		t.Errorf("ours' automation = %v", auto)
	}
}

func TestPerConflictResolution(t *testing.T) {
	base := fixture(t, "SampleAbletonProject_v2.als")
	a, b := fixture(t, "Split-A.als"), fixture(t, "Split-B.als")

	r := mustMerge(t, base, a, b, "fail")
	if len(r.Conflicts) != 1 || r.Conflicts[0].Key != "track:14" || !r.Conflicts[0].Unresolved {
		t.Fatalf("conflicts: %+v", r.Conflicts)
	}

	// Decide that one track: take theirs (Split-B's bounce track with Amp).
	res, err := MergeWith(base, a, b, Options{Strategy: "fail", Resolutions: map[string]string{"track:14": "theirs"}})
	if err != nil || len(res.Issues) > 0 {
		t.Fatal(err, res.Issues)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].Unresolved {
		t.Fatalf("resolved conflict still unresolved: %+v", res.Conflicts)
	}
	names := byName(res.Merged)["5 Bounce + Reverb"].DeviceNames()
	if len(names) != 2 || names[0] != "Amp" {
		t.Errorf("track 14 devices = %v, want theirs (Amp, Reverb)", names)
	}

	if _, err := MergeWith(base, a, b, Options{Strategy: "fail", Resolutions: map[string]string{"track:14": "mine"}}); err == nil {
		t.Error("invalid resolution accepted")
	}
}
