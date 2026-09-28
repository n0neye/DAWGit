// Package diff computes a semantic diff between two Live Sets. The unit of
// comparison is the track (matched by track Id, which Live keeps stable across
// saves) plus a handful of set-wide sections.
package diff

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"dawgit/internal/als"
	"dawgit/internal/xmltree"
)

// Section is a set-wide part of a Live Set compared as a whole.
type Section struct {
	Name  string
	Paths []string // under <LiveSet>
}

var GlobalSections = []Section{
	{"main", []string{"MainTrack"}},
	{"transport", []string{"Transport"}},
	{"locators", []string{"Locators"}},
	{"scenes", []string{"Scenes"}},
	{"grooves", []string{"GroovePool"}},
	{"sends_pre", []string{"SendsPre"}},
}

func SectionElems(s *als.LiveSet, sec Section) []*xmltree.Node {
	out := make([]*xmltree.Node, len(sec.Paths))
	for i, p := range sec.Paths {
		out[i] = s.LiveSet().Find(p)
	}
	return out
}

func SectionFingerprint(s *als.LiveSet, sec Section) string {
	var parts []string
	for _, e := range SectionElems(s, sec) {
		parts = append(parts, als.Fingerprint(e))
	}
	return strings.Join(parts, "|")
}

// Leaves flattens a subtree to {path@attr: value}, skipping noise. Same-tag
// siblings are disambiguated by occurrence index.
func Leaves(e *xmltree.Node) map[string]string {
	out := map[string]string{}
	leaves(e, "", out)
	return out
}

func leaves(e *xmltree.Node, prefix string, out map[string]string) {
	counts := map[string]int{}
	for _, c := range e.Children {
		if als.NoiseElements[c.Tag] {
			continue
		}
		n := counts[c.Tag]
		counts[c.Tag] = n + 1
		path := prefix + "/" + c.Tag
		if n > 0 {
			path += fmt.Sprintf("[%d]", n)
		}
		for _, a := range c.Attrs {
			if a.Name == "Id" || als.NoiseAttrs[a.Name] {
				continue
			}
			out[path+"@"+a.Name] = a.Value
		}
		if text := strings.TrimSpace(c.Text); text != "" {
			out[path+"#text"] = text
		}
		leaves(c, path, out)
	}
}

type TrackChange struct {
	TrackID, Name, Kind string
	Status              string // "added" | "removed" | "modified"
	Details             []string
}

type SetDiff struct {
	GlobalChanges []string
	TrackChanges  []TrackChange
	OrderChanged  bool
}

func (d *SetDiff) Empty() bool {
	return len(d.GlobalChanges) == 0 && len(d.TrackChanges) == 0 && !d.OrderChanged
}

func (d *SetDiff) Render() string {
	if d.Empty() {
		return "(no changes)"
	}
	var lines []string
	for _, g := range d.GlobalChanges {
		lines = append(lines, "~ "+g)
	}
	if d.OrderChanged {
		lines = append(lines, "~ track order changed")
	}
	sym := map[string]string{"added": "+", "removed": "-", "modified": "~"}
	for _, tc := range d.TrackChanges {
		lines = append(lines, fmt.Sprintf("%s %s \"%s\" (id %s)", sym[tc.Status], tc.Kind, tc.Name, tc.TrackID))
		for _, x := range tc.Details {
			lines = append(lines, "    "+x)
		}
	}
	return strings.Join(lines, "\n")
}

func db(gain string) string {
	g, _ := strconv.ParseFloat(gain, 64)
	if g > 0 {
		return fmt.Sprintf("%+.1f dB", 20*math.Log10(g))
	}
	return "-inf dB"
}

// pyG formats like Python's f"{x:g}".
func pyG(f float64) string { return strconv.FormatFloat(f, 'g', 6, 64) }

// pyFloatRepr approximates Python's repr(float) (used only for sort keys).
func pyFloatRepr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eInN") {
		s += ".0"
	}
	return s
}

func mixerDetails(a, b als.Track) []string {
	var out []string
	ma, mb := a.Elem.Find("DeviceChain/Mixer"), b.Elem.Find("DeviceChain/Mixer")
	if va, vb := ma.Val("Volume/Manual", ""), mb.Val("Volume/Manual", ""); va != vb {
		out = append(out, fmt.Sprintf("mixer volume: %s -> %s", db(va), db(vb)))
	}
	for _, p := range [][2]string{{"pan", "Pan/Manual"}, {"track on", "Speaker/Manual"}, {"solo", "SoloSink"}} {
		if va, vb := ma.Val(p[1], ""), mb.Val(p[1], ""); va != vb {
			out = append(out, fmt.Sprintf("mixer %s: %s -> %s", p[0], va, vb))
		}
	}
	sends := func(m *xmltree.Node) []string {
		var v []string
		for _, h := range m.FindAll("Sends/TrackSendHolder") {
			v = append(v, h.Val("Send/Manual", ""))
		}
		return v
	}
	if sa, sb := sends(ma), sends(mb); !equalStrings(sa, sb) {
		out = append(out, fmt.Sprintf("sends: %s -> %s", als.ReprStrings(sa), als.ReprStrings(sb)))
	}
	for _, p := range [][2]string{{"audio out", "DeviceChain/AudioOutputRouting/Target"},
		{"audio in", "DeviceChain/AudioInputRouting/Target"}, {"midi in", "DeviceChain/MidiInputRouting/Target"}} {
		if va, vb := a.Elem.Val(p[1], ""), b.Elem.Val(p[1], ""); va != vb {
			out = append(out, fmt.Sprintf("routing %s: %s -> %s", p[0], va, vb))
		}
	}
	return out
}

func deviceDetails(a, b als.Track) []string {
	da, dbs := a.Devices(), b.Devices()
	na, nb := a.DeviceNames(), b.DeviceNames()
	var out []string
	if !equalStrings(na, nb) {
		for _, op := range opcodes(na, nb) {
			if op.tag == "delete" || op.tag == "replace" {
				for _, n := range na[op.i1:op.i2] {
					out = append(out, "- device "+n)
				}
			}
			if op.tag == "insert" || op.tag == "replace" {
				for _, n := range nb[op.j1:op.j2] {
					out = append(out, "+ device "+n)
				}
			}
		}
		return out
	}
	for i := range da {
		devA, devB, label := da[i], dbs[i], na[i]
		if als.Fingerprint(devA) == als.Fingerprint(devB) {
			continue
		}
		if devA.Tag == "PluginDevice" {
			out = append(out, pluginDetails(devA, devB, label)...)
			continue
		}
		la, lb := Leaves(devA), Leaves(devB)
		paramSet := map[string]bool{}
		changed := 0
		for _, k := range unionKeys(la, lb) {
			va, oka := la[k]
			vb, okb := lb[k]
			if va == vb && oka == okb {
				continue
			}
			changed++
			if strings.HasSuffix(k, "/Manual@Value") {
				paramSet[strings.Split(k, "/")[1]] = true
			}
		}
		params := sortedKeys(paramSet)
		if len(params) > 0 {
			shown := strings.Join(params[:min(6, len(params))], ", ")
			if len(params) > 6 {
				shown += " ..."
			}
			out = append(out, fmt.Sprintf("~ device %s: params %s", label, shown))
		} else if changed > 0 {
			out = append(out, fmt.Sprintf("~ device %s: %d internal value(s) changed", label, changed))
		}
	}
	return out
}

func pluginDetails(a, b *xmltree.Node, label string) []string {
	var out []string
	params := func(d *xmltree.Node) map[string]string {
		m := map[string]string{}
		for _, p := range d.Iter("PluginFloatParameter") {
			m[p.Val("ParameterName", "")] = p.Val("ParameterValue/Manual", "")
		}
		return m
	}
	pa, pb := params(a), params(b)
	var changed []string
	for _, k := range unionKeys(pa, pb) {
		va, oka := pa[k]
		vb, okb := pb[k]
		if va != vb || oka != okb {
			changed = append(changed, k)
		}
	}
	if len(changed) > 0 {
		sort.Strings(changed)
		out = append(out, fmt.Sprintf("~ device %s: exposed params %s", label, strings.Join(changed[:min(6, len(changed))], ", ")))
	}
	state := func(d *xmltree.Node) []string {
		var v []string
		for _, e := range d.Iter("Buffer") {
			v = append(v, strings.TrimSpace(e.Text))
		}
		return v
	}
	if !equalStrings(state(a), state(b)) {
		out = append(out, fmt.Sprintf("~ device %s: plugin state changed (opaque)", label))
	}
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("~ device %s: settings changed", label))
	}
	return out
}

var clipParts = map[string]string{"Notes": "notes", "Fades": "fades", "Loop": "loop",
	"WarpMarkers": "warp markers", "Envelopes": "clip envelopes", "Name": "name", "Color": "color"}

// clipKey mirrors str() of the Python tuple used as sort key.
func clipKey(c als.Clip) string {
	return fmt.Sprintf("('%s', '%s', '%s', %s, %s)", c.Kind, c.Name, c.Location, pyFloatRepr(c.Start), pyFloatRepr(c.End))
}

func clipDetails(a, b als.Track) []string {
	index := func(t als.Track) map[string]als.Clip {
		m := map[string]als.Clip{}
		for _, c := range t.Clips() {
			m[clipKey(c)] = c
		}
		return m
	}
	ca, cb := index(a), index(b)
	var out []string
	for _, k := range sortedKeys(ca) {
		if _, ok := cb[k]; !ok {
			c := ca[k]
			out = append(out, fmt.Sprintf("- clip \"%s\" %s %s-%s", c.Name, c.Location, pyG(c.Start), pyG(c.End)))
		}
	}
	for _, k := range sortedKeys(cb) {
		if _, ok := ca[k]; !ok {
			c := cb[k]
			out = append(out, fmt.Sprintf("+ clip \"%s\" %s %s-%s", c.Name, c.Location, pyG(c.Start), pyG(c.End)))
		}
	}
	for _, k := range sortedKeys(ca) {
		y, ok := cb[k]
		if !ok {
			continue
		}
		x := ca[k]
		if als.Fingerprint(x.Elem) == als.Fingerprint(y.Elem) {
			continue
		}
		var parts []string
		for _, child := range y.Elem.Children {
			if als.Fingerprint(x.Elem.Child(child.Tag)) != als.Fingerprint(child) && !als.NoiseElements[child.Tag] {
				if p, ok := clipParts[child.Tag]; ok {
					parts = append(parts, p)
				} else {
					parts = append(parts, child.Tag)
				}
			}
		}
		what := strings.Join(parts, ", ")
		if what == "" {
			what = "content"
		}
		out = append(out, fmt.Sprintf("~ clip \"%s\" %s: %s changed", y.Name, y.Location, what))
	}
	return out
}

// ParamLabels maps pointee id -> label such as "Reverb: MixDirect" or "Mixer: Volume".
func ParamLabels(unit *xmltree.Node) map[string]string {
	labels := map[string]string{}
	var walk func(e *xmltree.Node, owner string)
	walk = func(e *xmltree.Node, owner string) {
		for _, c := range e.Children {
			o := owner
			switch {
			case e.Tag == "Devices":
				o = als.DeviceLabel(c)
			case c.Tag == "Mixer":
				o = "Mixer"
			case c.Tag == "TrackSendHolder":
				n, _ := strconv.Atoi(c.Attr("Id"))
				o = "Send " + string(rune('A'+n))
			}
			if c.Has("Id") && als.IsPointeeTag(c.Tag) {
				if o != "" {
					labels[c.Attr("Id")] = o + ": " + e.Tag
				} else {
					labels[c.Attr("Id")] = e.Tag
				}
			}
			walk(c, o)
		}
	}
	walk(unit, "")
	return labels
}

// Envelopes maps automated parameter label -> fingerprint of its automation.
func Envelopes(unit *xmltree.Node) map[string]string {
	out := map[string]string{}
	if unit == nil {
		return out
	}
	labels := ParamLabels(unit)
	envs := unit.Find("AutomationEnvelopes/Envelopes")
	if envs == nil {
		return out
	}
	for _, env := range envs.Children {
		pid := env.Val("EnvelopeTarget/PointeeId", "")
		label, ok := labels[pid]
		if !ok {
			label = "target " + pid
		}
		out[label] = als.Fingerprint(env.Child("Automation"))
	}
	return out
}

func automationDetails(a, b *xmltree.Node) []string {
	ea, eb := Envelopes(a), Envelopes(b)
	var out []string
	for _, k := range sortedKeys(ea) {
		if _, ok := eb[k]; !ok {
			out = append(out, "- automation "+k)
		}
	}
	for _, k := range sortedKeys(eb) {
		if _, ok := ea[k]; !ok {
			out = append(out, "+ automation "+k)
		}
	}
	for _, k := range sortedKeys(ea) {
		if v, ok := eb[k]; ok && v != ea[k] {
			out = append(out, "~ automation "+k)
		}
	}
	return out
}

func DiffTracks(a, b als.Track) []string {
	var details []string
	// EffectiveName also changes when Live renumbers auto-named tracks; only a
	// UserName change is a rename.
	if a.Elem.Val("Name/UserName", "") != b.Elem.Val("Name/UserName", "") {
		details = append(details, fmt.Sprintf("renamed: \"%s\" -> \"%s\"", a.Name(), b.Name()))
	}
	if a.GroupID() != b.GroupID() {
		details = append(details, fmt.Sprintf("group: %s -> %s", a.GroupID(), b.GroupID()))
	}
	details = append(details, deviceDetails(a, b)...)
	details = append(details, clipDetails(a, b)...)
	details = append(details, mixerDetails(a, b)...)
	details = append(details, automationDetails(a.Elem, b.Elem)...)
	if len(details) == 0 {
		details = append(details, "~ other changes")
	}
	return details
}

func Diff(a, b *als.LiveSet) *SetDiff {
	d := &SetDiff{}
	if a.Tempo() != b.Tempo() {
		d.GlobalChanges = append(d.GlobalChanges, fmt.Sprintf("tempo: %s -> %s", a.Tempo(), b.Tempo()))
	}
	for _, sec := range GlobalSections {
		if sec.Name == "main" && a.Tempo() != b.Tempo() {
			continue
		}
		if SectionFingerprint(a, sec) != SectionFingerprint(b, sec) {
			d.GlobalChanges = append(d.GlobalChanges, sec.Name+" changed")
		}
	}
	for _, x := range automationDetails(a.LiveSet().Child("MainTrack"), b.LiveSet().Child("MainTrack")) {
		d.GlobalChanges = append(d.GlobalChanges, "main track "+x)
	}

	ta, tb := a.TrackByID(), b.TrackByID()
	for _, t := range a.Tracks() {
		if _, ok := tb[t.ID()]; !ok {
			d.TrackChanges = append(d.TrackChanges, TrackChange{t.ID(), t.Name(), t.Kind(), "removed", nil})
		}
	}
	for _, t := range b.Tracks() {
		old, ok := ta[t.ID()]
		if !ok {
			devs := strings.Join(t.DeviceNames(), ", ")
			if devs == "" {
				devs = "(none)"
			}
			d.TrackChanges = append(d.TrackChanges, TrackChange{t.ID(), t.Name(), t.Kind(), "added", []string{"devices: " + devs}})
		} else if als.Fingerprint(old.Elem) != als.Fingerprint(t.Elem) {
			d.TrackChanges = append(d.TrackChanges, TrackChange{t.ID(), t.Name(), t.Kind(), "modified", DiffTracks(old, t)})
		}
	}

	var ca, cb []string
	for _, t := range a.Tracks() {
		if _, ok := tb[t.ID()]; ok {
			ca = append(ca, t.ID())
		}
	}
	for _, t := range b.Tracks() {
		if _, ok := ta[t.ID()]; ok {
			cb = append(cb, t.ID())
		}
	}
	d.OrderChanged = !equalStrings(ca, cb)
	return d
}

// --- helpers ---

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unionKeys(a, b map[string]string) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	return sortedKeys(seen)
}
