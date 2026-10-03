// Package als loads, inspects and saves Ableton Live Sets (.als).
//
// An .als file is gzip-compressed XML. Loading and saving is byte-exact: an
// untouched set saved back produces the identical XML payload Live wrote.
package als

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"dawgit/internal/store"
	"dawgit/internal/xmltree"
)

// TrackTags are the element tags of entries in LiveSet/Tracks.
var TrackTags = map[string]bool{"MidiTrack": true, "AudioTrack": true, "GroupTrack": true, "ReturnTrack": true}

// IsPointeeTag reports whether an element's Id lives in the set-wide pointee
// space (see NextPointeeId). Automation/modulation targets and controller
// targets are referenced by EnvelopeTarget/PointeeId, so their ids must be
// unique across the whole set.
func IsPointeeTag(tag string) bool {
	return strings.HasSuffix(tag, "Target") || tag == "Pointee" || strings.HasPrefix(tag, "ControllerTargets.")
}

type LiveSet struct {
	Root *xmltree.Node
	Path string
}

func Load(path string) (*LiveSet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := readGzip(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s.Path = path
	return s, nil
}

// FromGzip parses the bytes of an .als file.
func FromGzip(data []byte) (*LiveSet, error) { return readGzip(bytes.NewReader(data)) }

func readGzip(r io.Reader) (*LiveSet, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	return FromXML(data)
}

func FromXML(data []byte) (*LiveSet, error) {
	root, err := xmltree.Parse(data)
	if err != nil {
		return nil, err
	}
	if root.Child("LiveSet") == nil {
		return nil, fmt.Errorf("not a Live Set: missing <LiveSet>")
	}
	return &LiveSet{Root: root}, nil
}

func (s *LiveSet) XML() []byte { return xmltree.Serialize(s.Root) }

// Gzip returns the compressed .als payload (deterministic: no mtime/name).
func (s *LiveSet) Gzip() []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(s.XML())
	w.Close()
	return buf.Bytes()
}

func (s *LiveSet) Save(path string) error {
	// Whole or not at all: a set half written is a set lost.
	return store.WriteAtomic(path, bytes.NewReader(s.Gzip()))
}

// Clone returns an independent deep copy.
func (s *LiveSet) Clone() *LiveSet { return &LiveSet{Root: s.Root.Clone(), Path: s.Path} }

// --- accessors ---

func (s *LiveSet) LiveSet() *xmltree.Node { return s.Root.Child("LiveSet") }
func (s *LiveSet) Creator() string        { return s.Root.Attr("Creator") }
func (s *LiveSet) Tempo() string {
	return s.LiveSet().Val("MainTrack/DeviceChain/Mixer/Tempo/Manual", "")
}

func (s *LiveSet) NextPointeeID() int {
	n, _ := strconv.Atoi(s.LiveSet().Val("NextPointeeId", "0"))
	return n
}

func (s *LiveSet) SetNextPointeeID(v int) {
	s.LiveSet().Find("NextPointeeId").Set("Value", strconv.Itoa(v))
}

func (s *LiveSet) TracksElem() *xmltree.Node { return s.LiveSet().Child("Tracks") }

func (s *LiveSet) Tracks() []Track {
	var out []Track
	for _, e := range s.TracksElem().Children {
		if TrackTags[e.Tag] {
			out = append(out, Track{e})
		}
	}
	return out
}

func (s *LiveSet) TrackByID() map[string]Track {
	m := map[string]Track{}
	for _, t := range s.Tracks() {
		m[t.ID()] = t
	}
	return m
}

func (s *LiveSet) SceneCount() int { return len(s.LiveSet().Child("Scenes").Children) }

func (s *LiveSet) PointeeElements() []*xmltree.Node {
	var out []*xmltree.Node
	s.Root.Walk(func(e *xmltree.Node) bool {
		if e.Has("Id") && IsPointeeTag(e.Tag) {
			out = append(out, e)
		}
		return true
	})
	return out
}

type SampleRef struct {
	Path, RelativePath, RelativePathType, Pack, FileSize, CRC string
}

func (s *LiveSet) SampleRefs() []SampleRef {
	var out []SampleRef
	for _, sr := range s.Root.Iter("SampleRef") {
		fr := sr.Child("FileRef")
		if fr == nil {
			continue
		}
		out = append(out, SampleRef{
			Path:             fr.Val("Path", ""),
			RelativePath:     fr.Val("RelativePath", ""),
			RelativePathType: fr.Val("RelativePathType", ""),
			Pack:             fr.Val("LivePackName", ""),
			FileSize:         fr.Val("OriginalFileSize", ""),
			CRC:              fr.Val("OriginalCrc", ""),
		})
	}
	return out
}

func (s *LiveSet) Plugins() []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range s.Root.Iter("PluginDevice") {
		l := DeviceLabel(d)
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// --- tracks, devices, clips ---

type Track struct{ Elem *xmltree.Node }

func (t Track) Kind() string    { return t.Elem.Tag }
func (t Track) ID() string      { return t.Elem.Attr("Id") }
func (t Track) Name() string    { return t.Elem.Val("Name/EffectiveName", "") }
func (t Track) GroupID() string { return t.Elem.Val("TrackGroupId", "-1") }

func (t Track) Devices() []*xmltree.Node {
	if d := t.Elem.Find("DeviceChain/DeviceChain/Devices"); d != nil {
		return d.Children
	}
	return nil
}

func (t Track) DeviceNames() []string {
	var out []string
	for _, d := range t.Devices() {
		out = append(out, DeviceLabel(d))
	}
	return out
}

type Clip struct {
	Kind, Name string
	Start, End float64
	Location   string // "arrangement" | "session[<slot>]"
	Elem       *xmltree.Node
}

func (t Track) Clips() []Clip {
	var out []Clip
	seq := t.Elem.Find("DeviceChain/MainSequencer")
	if seq == nil {
		return out
	}
	for _, p := range []string{"ClipTimeable/ArrangerAutomation/Events", "Sample/ArrangerAutomation/Events"} {
		if events := seq.Find(p); events != nil {
			for _, c := range events.Children {
				if c.Tag == "MidiClip" || c.Tag == "AudioClip" {
					out = append(out, newClip(c, "arrangement"))
				}
			}
		}
	}
	if slots := seq.Child("ClipSlotList"); slots != nil {
		for i, slot := range slots.Children {
			if value := slot.Find("ClipSlot/Value"); value != nil {
				for _, c := range value.Children {
					if c.Tag == "MidiClip" || c.Tag == "AudioClip" {
						out = append(out, newClip(c, fmt.Sprintf("session[%d]", i)))
					}
				}
			}
		}
	}
	return out
}

func newClip(c *xmltree.Node, location string) Clip {
	start, _ := strconv.ParseFloat(c.Val("CurrentStart", "0"), 64)
	end, _ := strconv.ParseFloat(c.Val("CurrentEnd", "0"), 64)
	return Clip{Kind: c.Tag, Name: c.Val("Name", ""), Start: start, End: end, Location: location, Elem: c}
}

// DeviceLabel is a human label: plugin name and format, or device class plus
// user preset name.
func DeviceLabel(d *xmltree.Node) string {
	if d.Tag == "PluginDevice" {
		var plugin *xmltree.Node
		if info := d.Child("PluginDesc"); info != nil && len(info.Children) > 0 {
			plugin = info.Children[0]
		}
		name := plugin.Val("Name", "")
		if name == "" {
			name = plugin.Val("PlugName", "")
		}
		if name == "" {
			name = "?"
		}
		format := "Plugin"
		if plugin != nil {
			format = strings.ReplaceAll(plugin.Tag, "PluginInfo", "")
		}
		return fmt.Sprintf("%s (%s)", name, format)
	}
	if user := d.Val("UserName", ""); user != "" {
		return d.Tag + ` "` + user + `"` // not %q: keep non-ASCII names as-is
	}
	return d.Tag
}

// PluginRef is a third-party plugin a set uses.
type PluginRef struct {
	Name   string // as Live shows it
	Format string // "VST3", "VST" (VST2) or "AU"
	// UID identifies a VST3 plugin as Live's plugin list does
	// (8-4-4-4-12 hex digits); VST2: its unique id (decimal).
	UID  string
	File string // VST2: the plugin's file name (e.g. "Serum_x64.dll")
}

// PluginRefs lists the plugins the set uses, each once.
func (s *LiveSet) PluginRefs() []PluginRef {
	seen := map[string]bool{}
	var out []PluginRef
	for _, d := range s.Root.Iter("PluginDevice") {
		desc := d.Child("PluginDesc")
		if desc == nil || len(desc.Children) == 0 {
			continue
		}
		info := desc.Children[0]
		p := PluginRef{Name: info.Val("Name", ""), Format: strings.TrimSuffix(info.Tag, "PluginInfo")}
		switch info.Tag {
		case "Vst3PluginInfo":
			p.Format = "VST3"
			for _, u := range info.Iter("Uid") {
				p.UID = vst3UID(u)
				break
			}
		case "VstPluginInfo":
			p.Format = "VST"
			p.Name = info.Val("PlugName", "")
			p.UID = info.Val("UniqueId", "")
			if path := info.Val("Path", ""); path != "" {
				p.File = path[strings.LastIndexAny(path, `/\`)+1:]
			}
		case "AuPluginInfo":
			p.Format = "AU"
		}
		if p.Name == "" {
			p.Name = info.Val("PlugName", "?")
		}
		key := p.Format + "|" + p.UID + "|" + p.Name
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// vst3UID writes a VST3 class id stored as four 32-bit numbers the way
// Live's plugin list does: 56535456-6974-6176-6974-616c00000000.
func vst3UID(u *xmltree.Node) string {
	var b []byte
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseInt(u.Val(fmt.Sprintf("Fields.%d", i), "0"), 10, 64)
		if err != nil {
			return ""
		}
		x := uint32(v)
		b = append(b, byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
	}
	h := fmt.Sprintf("%x", b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// Version is the Live version that saved the set ("12.1.5"), from Creator.
func (s *LiveSet) Version() string {
	c := s.Creator()
	if i := strings.LastIndex(c, " "); i >= 0 {
		return c[i+1:]
	}
	return c
}
