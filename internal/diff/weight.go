package diff

import "strings"

// Weights: how much a change matters to the music, from least to most. The
// app shows small changes quietly and big ones loudly; a plugin re-saving
// its own state (many do whenever a set is saved) is almost never the user.
const (
	WeightNoise       = "noise"       // a plugin's opaque state changed by itself
	WeightTidy        = "tidy"        // names, colors, order, groups, locators
	WeightMix         = "mix"         // volume, pan, sends, routing, automation
	WeightSound       = "sound"       // devices added or removed, their settings
	WeightArrangement = "arrangement" // clips, notes, tracks added or removed, tempo, scenes
)

var weightRank = map[string]int{"": -1, WeightNoise: 0, WeightTidy: 1, WeightMix: 2, WeightSound: 3, WeightArrangement: 4}

// Heavier is the weightier of a and b.
func Heavier(a, b string) string {
	if weightRank[b] > weightRank[a] {
		return b
	}
	return a
}

// DetailWeight weighs one line of TrackChange.Details.
func DetailWeight(d string) string {
	switch {
	case strings.HasPrefix(d, "renamed:"), strings.HasPrefix(d, "group:"), d == "~ other changes":
		return WeightTidy
	case strings.HasPrefix(d, "+ device "), strings.HasPrefix(d, "- device "):
		return WeightSound
	case strings.HasPrefix(d, "~ device "):
		if strings.HasSuffix(d, ": plugin state changed (opaque)") {
			return WeightNoise
		}
		return WeightSound
	case strings.HasPrefix(d, "+ clip "), strings.HasPrefix(d, "- clip "):
		return WeightArrangement
	case strings.HasPrefix(d, "~ clip "):
		// Only its name or color: tidying.
		what := d[strings.LastIndex(d, ": ")+2:]
		what = strings.TrimSuffix(what, " changed")
		for _, part := range strings.Split(what, ", ") {
			if part != "name" && part != "color" {
				return WeightArrangement
			}
		}
		return WeightTidy
	case strings.HasPrefix(d, "mixer "), strings.HasPrefix(d, "sends:"), strings.HasPrefix(d, "routing "),
		strings.HasPrefix(d, "+ automation "), strings.HasPrefix(d, "- automation "), strings.HasPrefix(d, "~ automation "):
		return WeightMix
	case strings.HasPrefix(d, "devices:"): // a new track's devices
		return WeightArrangement
	}
	return WeightTidy
}

// Weight weighs a track's change: a track added or removed is arrangement,
// otherwise its heaviest detail.
func (tc TrackChange) Weight() string {
	if tc.Status != "modified" {
		return WeightArrangement
	}
	w := ""
	for _, d := range tc.Details {
		w = Heavier(w, DetailWeight(d))
	}
	if w == "" {
		w = WeightTidy
	}
	return w
}

// GlobalWeight weighs one line of SetDiff.GlobalChanges.
func GlobalWeight(g string) string {
	switch {
	case strings.HasPrefix(g, "tempo:"), g == "scenes changed":
		return WeightArrangement
	case g == "grooves changed":
		return WeightSound
	case g == "main changed", g == "sends_pre changed", strings.HasPrefix(g, "main track "):
		return WeightMix
	}
	return WeightTidy // locators, transport
}

// Weight is the heaviest change in the set.
func (d *SetDiff) Weight() string {
	w := ""
	if d.OrderChanged {
		w = WeightTidy
	}
	for _, g := range d.GlobalChanges {
		w = Heavier(w, GlobalWeight(g))
	}
	for _, tc := range d.TrackChanges {
		w = Heavier(w, tc.Weight())
	}
	return w
}
