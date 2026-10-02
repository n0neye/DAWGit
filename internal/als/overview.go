package als

import (
	"math"
	"strconv"
	"strings"

	"dawgit/internal/xmltree"
)

// Overview is what a set looks like at a glance: its tracks as Live shows
// them in Arrangement and Session view, without the devices' insides.
type Overview struct {
	Creator  string         `json:"creator"`
	Tempo    float64        `json:"tempo"`
	TimeSig  [2]int         `json:"timeSig"` // numerator, denominator
	Length   float64        `json:"length"`  // beats: the end of the last arrangement clip
	Scenes   []string       `json:"scenes"`
	Locators []Locator      `json:"locators"`
	Tracks   []TrackSummary `json:"tracks"`
	Main     TrackSummary   `json:"main"`
}

type Locator struct {
	Name string  `json:"name"`
	Time float64 `json:"time"` // beats
}

type TrackSummary struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Kind    string        `json:"kind"`  // midi | audio | group | return | main
	Color   int           `json:"color"` // Live's color index (0-69), -1 for none
	Group   string        `json:"group"` // the group track's id, "" when in none
	Folded  bool          `json:"folded"`
	Volume  float64       `json:"volume"` // dB; -inf is -1000
	Muted   bool          `json:"muted"`  // the track activator is off
	Solo    bool          `json:"solo"`
	Devices []string      `json:"devices"`
	Clips   []ClipSummary `json:"clips"`
}

type ClipSummary struct {
	Name     string  `json:"name"`
	Color    int     `json:"color"`
	Start    float64 `json:"start"` // beats (arrangement)
	End      float64 `json:"end"`
	Slot     int     `json:"slot"`     // session: the scene's index; -1 in the arrangement
	Disabled bool    `json:"disabled"` // deactivated clip
}

var kinds = map[string]string{"MidiTrack": "midi", "AudioTrack": "audio", "GroupTrack": "group", "ReturnTrack": "return"}

// Overview summarizes the set.
func (s *LiveSet) Overview() *Overview {
	ls := s.LiveSet()
	o := &Overview{Creator: s.Creator(), TimeSig: [2]int{4, 4}, Scenes: []string{}, Locators: []Locator{}, Tracks: []TrackSummary{}}
	o.Tempo, _ = strconv.ParseFloat(s.Tempo(), 64)
	main := ls.Child("MainTrack")
	if main == nil {
		main = ls.Child("MasterTrack") // before Live 12
	}
	if main != nil {
		o.Main = summarize(main, "main", "")
		o.Main.Name = "Main"
		if v, err := strconv.Atoi(main.Val("DeviceChain/Mixer/TimeSignature/Manual", "")); err == nil {
			o.TimeSig = timeSignature(v)
		}
	}
	for _, sc := range ls.FindAll("Scenes/Scene") {
		o.Scenes = append(o.Scenes, sc.Val("Name", ""))
	}
	for _, l := range ls.FindAll("Locators/Locators/Locator") {
		t, _ := strconv.ParseFloat(l.Val("Time", "0"), 64)
		o.Locators = append(o.Locators, Locator{Name: l.Val("Name", ""), Time: t})
	}
	for _, t := range s.Tracks() {
		ts := summarize(t.Elem, kinds[t.Kind()], t.ID())
		ts.Name = t.Name()
		if g := t.GroupID(); g != "-1" {
			ts.Group = g
		}
		ts.Devices = t.DeviceNames()
		for _, c := range t.Clips() {
			cs := ClipSummary{Name: c.Name, Color: color(c.Elem), Start: c.Start, End: c.End, Slot: -1,
				Disabled: c.Elem.Val("Disabled", "false") == "true"}
			if strings.HasPrefix(c.Location, "session[") {
				cs.Slot, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(c.Location, "session["), "]"))
			} else if c.End > o.Length {
				o.Length = c.End
			}
			ts.Clips = append(ts.Clips, cs)
		}
		o.Tracks = append(o.Tracks, ts)
	}
	return o
}

func summarize(e *xmltree.Node, kind, id string) TrackSummary {
	mixer := e.Find("DeviceChain/Mixer")
	t := TrackSummary{ID: id, Kind: kind, Color: color(e), Devices: []string{}, Clips: []ClipSummary{}}
	t.Folded = e.Val("TrackUnfolded", "true") == "false" && kind == "group"
	gain, err := strconv.ParseFloat(mixer.Val("Volume/Manual", "1"), 64)
	if err != nil {
		gain = 1
	}
	t.Volume = -1000
	if gain > 0 {
		t.Volume = math.Round(200*math.Log10(gain)) / 10
	}
	t.Muted = mixer.Val("Speaker/Manual", "true") == "false"
	t.Solo = mixer.Val("SoloSink", "false") == "true"
	return t
}

// color reads Live's color index: Color (Live 11 on) or ColorIndex.
func color(e *xmltree.Node) int {
	for _, tag := range []string{"Color", "ColorIndex"} {
		if c := e.Child(tag); c != nil {
			if v, err := strconv.Atoi(c.Attr("Value")); err == nil {
				return v
			}
		}
	}
	return -1
}

// timeSignature decodes Live's time signature value: numerator-1 plus 99
// times log2 of the denominator (201 is 4/4).
func timeSignature(v int) [2]int {
	if v < 0 {
		return [2]int{4, 4}
	}
	return [2]int{v%99 + 1, 1 << (v / 99)}
}
