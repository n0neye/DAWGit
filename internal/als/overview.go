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
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Named   bool     `json:"named"` // the name was given (not one Live makes up and renumbers)
	Kind    string   `json:"kind"`  // midi | audio | group | return | main
	Color   int      `json:"color"` // Live's color index (0-69), -1 for none
	Group   string   `json:"group"` // the group track's id, "" when in none
	Folded  bool     `json:"folded"`
	Volume  float64  `json:"volume"` // dB; -inf is -1000
	Muted   bool     `json:"muted"`  // the track activator is off
	Solo    bool     `json:"solo"`
	Devices []string `json:"devices"`
	// Instrument: the instrument a MIDI track plays (Vital, Sampler, ...);
	// InstrumentFull says where it is (in a rack, with its preset name).
	Instrument     string        `json:"instrument"`
	InstrumentFull string        `json:"instrumentFull"`
	Clips          []ClipSummary `json:"clips"`
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
		ts.Named = t.Elem.Val("Name/UserName", "") != ""
		if g := t.GroupID(); g != "-1" {
			ts.Group = g
		}
		ts.Devices = t.DeviceNames()
		if t.Kind() == "MidiTrack" {
			ts.Instrument, ts.InstrumentFull = instrument(t.Devices())
		}
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

// Live's instruments by element tag, as Live names them.
var instrumentNames = map[string]string{
	"UltraAnalog": "Analog", "OriginalSimpler": "Simpler", "MultiSampler": "Sampler", "Operator": "Operator",
	"Collision": "Collision", "StringStudio": "Tension", "LoungeLizard": "Electric", "InstrumentVector": "Wavetable",
	"Drift": "Drift", "InstrumentMeld": "Meld", "InstrumentImpulse": "Impulse", "DrumGroupDevice": "Drum Rack",
	"ExternalInstrument": "External Instrument", "MxDeviceInstrument": "Max Instrument", "InstrumentGroupDevice": "Instrument Rack",
	"Bass": "Bass", "Poli": "Poli",
}

// instrument finds the instrument in a MIDI track's devices: the first
// device after any MIDI effects; inside an Instrument Rack, the one its
// first chain plays.
func instrument(devices []*xmltree.Node) (name, full string) {
	for _, d := range devices {
		if strings.HasPrefix(d.Tag, "Midi") || d.Tag == "MxDeviceMidiEffect" {
			continue // a MIDI effect before the instrument
		}
		switch d.Tag {
		case "PluginDevice":
			label := DeviceLabel(d)
			name = strings.TrimSpace(label[:strings.LastIndex(label, "(")])
			return name, label
		case "InstrumentGroupDevice":
			rack := "Instrument Rack"
			if u := d.Val("UserName", ""); u != "" {
				rack += ` "` + u + `"`
			}
			for _, chain := range d.Iter("InstrumentBranch") {
				if inner := chain.Find("DeviceChain/MidiToAudioDeviceChain/Devices"); inner != nil {
					if n, f := instrument(inner.Children); n != "" {
						return n, rack + ": " + f
					}
				}
			}
			return "Instrument Rack", rack
		case "MxDeviceInstrument":
			if u := d.Val("UserName", ""); u != "" {
				return u, "Max for Live: " + u
			}
		}
		if n, ok := instrumentNames[d.Tag]; ok {
			full := n
			if u := d.Val("UserName", ""); u != "" {
				full += ` "` + u + `"`
			}
			return n, full
		}
		return "", "" // an audio effect first: no instrument
	}
	return "", ""
}
