package profile

import "testing"

func TestAddAndRemoveRule(t *testing.T) {
	text := "presets:\n  ./: ableton  # found by DAWGit\nrules:\n  # Later rules win.\n  - ignore: \"Exports/\"\n  - track: \"*.asd\"\n"
	got, err := AddRule(text, "track", "/Exports/Final/")
	if want := text + "  - track: \"/Exports/Final/\"\n"; err != nil || got != want {
		t.Fatalf("add: %v\n%q\nwant\n%q", err, got, want)
	}
	// The same rule again: moved to the end (it decides last).
	got, _ = AddRule(text, "ignore", "Exports/")
	if want := "presets:\n  ./: ableton  # found by DAWGit\nrules:\n  # Later rules win.\n  - track: \"*.asd\"\n  - ignore: \"Exports/\"\n"; got != want {
		t.Errorf("again:\n%q\nwant\n%q", got, want)
	}
	got, err = RemoveRule(text, 0)
	if want := "presets:\n  ./: ableton  # found by DAWGit\nrules:\n  # Later rules win.\n  - track: \"*.asd\"\n"; err != nil || got != want {
		t.Errorf("remove: %v\n%q", err, got)
	}
	if _, err := RemoveRule(text, 2); err == nil {
		t.Error("no third rule")
	}
	p, err := Parse([]byte(got), t.TempDir())
	if err != nil || len(p.Rules) != 1 || p.Rules[0].Track != "*.asd" {
		t.Errorf("parsed: %+v %v", p.Rules, err)
	}
}

func TestPresetEntries(t *testing.T) {
	text := "presets:\n  ./: design  # found by DAWGit\n  \"Game/\": unity  # found by DAWGit\n  Music/Theme Project/: ableton\n  # a note\n  \"Tools/\": none # mine\nrules:\n"
	got := PresetEntries(text)
	want := []PresetEntry{{"", "design", true}, {"Game", "unity", true}, {"Music/Theme Project", "ableton", false}, {"Tools", "none", false}}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
}
