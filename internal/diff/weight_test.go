package diff

import "testing"

func TestDetailWeight(t *testing.T) {
	for d, want := range map[string]string{
		`renamed: "Bass" -> "Sub Bass"`:                           WeightTidy,
		"group: -1 -> 15":                                         WeightTidy,
		"~ other changes":                                         WeightTidy,
		"+ device Reverb":                                         WeightSound,
		"~ device Vital (Vst3): exposed params Cutoff":            WeightSound,
		"~ device Vital (Vst3): settings changed":                 WeightSound,
		"~ device Vital (Vst3): plugin state changed (opaque)":    WeightNoise,
		`+ clip "Fill" session[2] 0-8`:                            WeightArrangement,
		`~ clip "Fill" arrangement: notes changed`:                WeightArrangement,
		`~ clip "Fill" arrangement: name, color changed`:          WeightTidy,
		`~ clip "Fill" arrangement: color changed`:                WeightTidy,
		"mixer volume: 0.0 dB -> -3.0 dB":                         WeightMix,
		"sends: ['1'] -> ['0.5']":                                 WeightMix,
		"routing audio out: AudioOut/Main -> AudioOut/GroupTrack": WeightMix,
		"+ automation Mixer: Volume":                              WeightMix,
		"~ automation Reverb: MixDirect":                          WeightMix,
	} {
		if got := DetailWeight(d); got != want {
			t.Errorf("%q: %s, want %s", d, got, want)
		}
	}
}

func TestSetWeight(t *testing.T) {
	plugin := TrackChange{Status: "modified", Details: []string{"~ device Vital (Vst3): plugin state changed (opaque)"}}
	if plugin.Weight() != WeightNoise {
		t.Error("plugin state alone is noise")
	}
	d := &SetDiff{OrderChanged: true, TrackChanges: []TrackChange{plugin}}
	if d.Weight() != WeightTidy {
		t.Errorf("order + plugin noise: %s", d.Weight())
	}
	d.GlobalChanges = []string{"tempo: 120 -> 124"}
	if d.Weight() != WeightArrangement {
		t.Errorf("tempo: %s", d.Weight())
	}
	if (TrackChange{Status: "added"}).Weight() != WeightArrangement {
		t.Error("a new track is arrangement")
	}
	if (&SetDiff{}).Weight() != "" {
		t.Error("nothing changed: no weight")
	}
}
