// Package merge implements a track-level 3-way merge of Live Sets.
//
// Units of merge:
//   - each track (matched by track Id; Live keeps it stable across saves)
//   - each (track, return track) send, merged independently from the track body
//   - each track's placement (group membership and output routing)
//   - track order
//   - set-wide sections: main track (tempo, master chain), transport,
//     locators, scenes, groove pool
//
// After units are chosen the result is repaired so Live can load it: send and
// clip-slot lists are rebuilt to match return tracks / scenes, pointee ids that
// collide are renumbered (with their envelope references), dangling automation
// is dropped and NextPointeeId is advanced.
//
// This is a port of python/dawgit/merge.py and must produce byte-identical
// output; keep the two in step.
package merge

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"dawgit/internal/als"
	"dawgit/internal/diff"
	"dawgit/internal/xmltree"
)

var Strategies = []string{"fail", "ours", "theirs", "both"}

// Sends are merged per (track, return); placement separately; clip slots are
// compared by content only.
var (
	placementPaths = []string{"TrackGroupId", "DeviceChain/AudioOutputRouting"}
	trackSkip      = []string{"Sends", "ClipSlotList", "Slots", "TrackGroupId", "AudioOutputRouting"}
	autoNumberRE   = regexp.MustCompile(`^\d+-`)
)

type Conflict struct {
	// Key identifies the merge unit, stable for the same inputs:
	// "track:<id>", "placement:<id>", "send:<track>:<return>", "section:<name>"
	// or "order". Pass it back in Options.Resolutions to decide this conflict.
	Key                           string
	Unit, Description, Resolution string
	// Unresolved is set when the strategy for this conflict was "fail".
	Unresolved bool
}

// Options control conflict resolution.
type Options struct {
	// Strategy applies to conflicts without an entry in Resolutions:
	// fail | ours | theirs | both.
	Strategy string
	// Resolutions decide individual conflicts by Conflict.Key.
	Resolutions map[string]string
}

type Result struct {
	Merged    *als.LiveSet
	Log       []string
	Conflicts []Conflict
	Issues    []string
}

func (r *Result) Report() string {
	var lines []string
	for _, l := range r.Log {
		lines = append(lines, "  "+l)
	}
	if len(lines) == 0 {
		lines = []string{"  (nothing to merge)"}
	}
	if len(r.Conflicts) > 0 {
		lines = append(lines, fmt.Sprintf("\n%d conflict(s):", len(r.Conflicts)))
		for _, c := range r.Conflicts {
			lines = append(lines, fmt.Sprintf("  ! %s: %s -> %s", c.Unit, c.Description, c.Resolution))
		}
	}
	if len(r.Issues) > 0 {
		lines = append(lines, "\nvalidation issues:")
		for _, i := range r.Issues {
			lines = append(lines, "  x "+i)
		}
	}
	return strings.Join(lines, "\n")
}

// Merge performs a 3-way merge; ours is the starting point and theirs' changes
// are applied on top.
func Merge(base, ours, theirs *als.LiveSet, strategy string) (*Result, error) {
	return MergeWith(base, ours, theirs, Options{Strategy: strategy})
}

// MergeWith is Merge with per-conflict resolutions.
func MergeWith(base, ours, theirs *als.LiveSet, opts Options) (*Result, error) {
	valid := func(s string) bool {
		for _, x := range Strategies {
			if s == x {
				return true
			}
		}
		return false
	}
	if !valid(opts.Strategy) {
		return nil, fmt.Errorf("unknown strategy %q", opts.Strategy)
	}
	for k, v := range opts.Resolutions {
		if !valid(v) {
			return nil, fmt.Errorf("unknown resolution %q for %s", v, k)
		}
	}
	m := newMerger(base, ours, theirs, opts)
	m.run()
	return m.result, nil
}

// none marks an absent unit in 3-way comparisons (fingerprints are never empty).
const none = ""

func threeWay(fb, fo, ft string) string {
	if fo == ft || ft == fb {
		return "ours"
	}
	if fo == fb {
		return "theirs"
	}
	return "conflict"
}

func placementFP(e *xmltree.Node) string {
	parts := make([]string, len(placementPaths))
	for i, p := range placementPaths {
		parts[i] = als.Fingerprint(e.Find(p))
	}
	return strings.Join(parts, "|")
}

// parentOf returns the parent of the element at a slash path relative to e.
func parentOf(e *xmltree.Node, path string) *xmltree.Node {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return e.Find(path[:i])
	}
	return e
}

// replaceChild replaces the element at path with n, keeping position and tail.
func replaceChild(e *xmltree.Node, path string, n *xmltree.Node) {
	parentOf(e, path).Replace(e.Find(path), n)
}

// replaceAt swaps old for n keeping position; n keeps its own tail.
func replaceAt(parent, old, n *xmltree.Node) {
	i := parent.IndexOf(old)
	parent.Children[i] = n
}

// retail re-indents children after structural edits (Live uses tabs).
func retail(parent *xmltree.Node) {
	if len(parent.Children) == 0 {
		parent.Text = ""
		return
	}
	indent := "\n"
	if parent.Text != "" && strings.TrimSpace(parent.Text) == "" {
		indent = parent.Text
	}
	for _, k := range parent.Children {
		k.Tail = indent
	}
	last := indent
	if strings.HasSuffix(indent, "\t") {
		last = indent[:len(indent)-1]
	}
	parent.Children[len(parent.Children)-1].Tail = last
}

// ident says which side's track is the same logical track: side -> track id.
type ident map[string]string

func (id ident) only(side string) bool {
	_, ok := id[side]
	return ok && len(id) == 1
}

type merger struct {
	base, ours, theirs *als.LiveSet
	sides              map[string]*als.LiveSet
	strategy           string
	resolutions        map[string]string
	merged             *als.LiveSet
	result             *Result

	// Nodes copied from theirs or synthesized; their pointee ids yield to
	// ours' on collision. Recorded at copy time because ours' nodes may later
	// be attached under an imported track (e.g. send holders).
	imported map[*xmltree.Node]bool

	// merged track element -> ident, in insertion order (mirrors a Python dict).
	identOrder []*xmltree.Node
	idents     map[*xmltree.Node]ident

	theirsRenumber map[string]string // theirs track id -> new id
	nextTrackID    int

	byID  map[string]map[string]als.Track // side -> id -> track (sides never change)
	fpMem map[*xmltree.Node]string
}

func newMerger(base, ours, theirs *als.LiveSet, opts Options) *merger {
	m := &merger{
		base: base, ours: ours, theirs: theirs,
		sides:          map[string]*als.LiveSet{"base": base, "ours": ours, "theirs": theirs},
		strategy:       opts.Strategy,
		resolutions:    opts.Resolutions,
		merged:         ours.Clone(),
		imported:       map[*xmltree.Node]bool{},
		idents:         map[*xmltree.Node]ident{},
		theirsRenumber: map[string]string{},
		byID:           map[string]map[string]als.Track{},
		fpMem:          map[*xmltree.Node]string{},
	}
	m.result = &Result{Merged: m.merged}
	for name, s := range m.sides {
		m.byID[name] = s.TrackByID()
		for _, t := range s.Tracks() {
			if n, err := strconv.Atoi(t.ID()); err == nil {
				m.nextTrackID = max(m.nextTrackID, n+1)
			}
		}
	}
	if m.nextTrackID == 0 {
		m.nextTrackID = 1
	}
	return m
}

// --- helpers ---

func (m *merger) log(format string, a ...any) {
	m.result.Log = append(m.result.Log, fmt.Sprintf(format, a...))
}

func (m *merger) conflict(key, unit, description, resolution string) {
	m.result.Conflicts = append(m.result.Conflicts, Conflict{Key: key, Unit: unit, Description: description,
		Resolution: resolution, Unresolved: m.choice(key) == "fail"})
}

// choice is the strategy for one conflict.
func (m *merger) choice(key string) string {
	if r, ok := m.resolutions[key]; ok {
		return r
	}
	return m.strategy
}

func (m *merger) importNode(e *xmltree.Node) *xmltree.Node {
	c := e.Clone()
	c.Walk(func(n *xmltree.Node) bool { m.imported[n] = true; return true })
	return c
}

func (m *merger) setIdent(e *xmltree.Node, id ident) {
	if _, ok := m.idents[e]; !ok {
		m.identOrder = append(m.identOrder, e)
	}
	m.idents[e] = id
}

func (m *merger) popIdent(e *xmltree.Node) ident {
	id, ok := m.idents[e]
	if !ok {
		return nil
	}
	delete(m.idents, e)
	for i, x := range m.identOrder {
		if x == e {
			m.identOrder = append(m.identOrder[:i], m.identOrder[i+1:]...)
			break
		}
	}
	return id
}

func (m *merger) identOf(t als.Track) ident {
	if id, ok := m.idents[t.Elem]; ok {
		return id
	}
	return m.identFor(t.ID())
}

// trackFP is the track identity for 3-way decisions. Empty clip slots are left
// out so adding/removing a scene on one side does not modify every track.
func (m *merger) trackFP(t *als.Track) string {
	if t == nil {
		return none
	}
	if fp, ok := m.fpMem[t.Elem]; ok {
		return fp
	}
	parts := []string{als.Fingerprint(t.Elem, trackSkip...)}
	for _, seq := range []string{"MainSequencer", "FreezeSequencer"} {
		slots := t.Elem.Find("DeviceChain/" + seq + "/ClipSlotList")
		if slots == nil {
			continue
		}
		for i, slot := range slots.Children {
			if v := slot.Find("ClipSlot/Value"); v != nil && len(v.Children) > 0 {
				parts = append(parts, fmt.Sprintf("%s[%d]=%s", seq, i, als.Fingerprint(slot)))
			}
		}
	}
	fp := strings.Join(parts, "|")
	m.fpMem[t.Elem] = fp
	return fp
}

func (m *merger) track(side, id string) *als.Track {
	if t, ok := m.byID[side][id]; ok {
		return &t
	}
	return nil
}

// identFor: ids are only a shared identity when the track existed in base; two
// tracks created independently on each side can share an id by chance.
func (m *merger) identFor(tid string) ident {
	id := ident{}
	o, b, t := m.track("ours", tid), m.track("base", tid), m.track("theirs", tid)
	if o != nil {
		id["ours"] = tid
	}
	if b != nil {
		id["base"] = tid
	}
	if t != nil && (b != nil || o == nil || m.trackFP(o) == m.trackFP(t)) {
		id["theirs"] = tid
	}
	return id
}

func (m *merger) newTrackID() string {
	id := strconv.Itoa(m.nextTrackID)
	m.nextTrackID++
	return id
}

// resolve picks a side for a conflict that cannot keep both.
func (m *merger) resolve(key string) string {
	if m.choice(key) == "theirs" {
		return "theirs"
	}
	return "ours"
}

// --- steps ---

func (m *merger) mergeGlobals() {
	for _, sec := range diff.GlobalSections {
		if sec.Name == "sends_pre" {
			continue // rebuilt from the merged return tracks
		}
		decision := threeWay(diff.SectionFingerprint(m.base, sec), diff.SectionFingerprint(m.ours, sec),
			diff.SectionFingerprint(m.theirs, sec))
		if decision == "conflict" {
			key := "section:" + sec.Name
			decision = m.resolve(key)
			m.conflict(key, sec.Name, "changed on both sides", decision)
		} else if decision == "theirs" {
			m.log("%s: took theirs", sec.Name)
		}
		if decision == "theirs" {
			ls := m.merged.LiveSet()
			mine, their := diff.SectionElems(m.merged, sec), diff.SectionElems(m.theirs, sec)
			for i := range mine {
				replaceAt(ls, mine[i], m.importNode(their[i]))
			}
		}
	}
}

func (m *merger) mergeTracks() {
	tracksElem := m.merged.TracksElem()
	mergedByID := map[string]*xmltree.Node{}
	for _, t := range m.merged.Tracks() {
		mergedByID[t.ID()] = t.Elem
		m.setIdent(t.Elem, m.identFor(t.ID()))
	}

	// Tracks theirs created with an id ours also used for a different new
	// track get a fresh id. Known up front so references inside every
	// imported theirs track can be rewritten at copy time.
	for _, t := range m.theirs.Tracks() {
		tid := t.ID()
		if m.track("base", tid) == nil {
			if o := m.track("ours", tid); o != nil && m.trackFP(o) != m.trackFP(&t) {
				m.theirsRenumber[tid] = m.newTrackID()
			}
		}
	}

	// theirs track id -> merged element representing it (for placement)
	placed := map[string]*xmltree.Node{}
	for tid, e := range mergedByID {
		if _, ok := m.idents[e]["theirs"]; ok {
			placed[tid] = e
		}
	}
	var theirsOrder []string
	for _, t := range m.theirs.Tracks() {
		theirsOrder = append(theirsOrder, t.ID())
	}

	insertAfterTheirsNeighbour := func(tid string, e *xmltree.Node) {
		kids := tracksElem.Children
		idx := 0
		pos := indexOf(theirsOrder, tid)
		for k := pos - 1; k >= 0; k-- {
			if p, ok := placed[theirsOrder[k]]; ok {
				if i := tracksElem.IndexOf(p); i >= 0 {
					idx = i + 1
					break
				}
			}
		}
		// Tracks ours added at the same spot come first.
		for idx < len(kids) && m.idents[kids[idx]].only("ours") {
			idx++
		}
		tracksElem.Insert(idx, e)
		placed[tid] = e
	}

	importTheirs := func(tid string) *xmltree.Node {
		e := m.importNode(m.byID["theirs"][tid].Elem)
		if g := e.Child("TrackGroupId"); g != nil {
			if n, ok := m.theirsRenumber[g.Attr("Value")]; ok {
				g.Set("Value", n)
			}
		}
		for _, target := range e.Iter("Target") {
			v := target.Attr("Value")
			if loc := als.RoutingTrackRE.FindStringSubmatchIndex(v); loc != nil {
				if n, ok := m.theirsRenumber[v[loc[2]:loc[3]]]; ok {
					target.Set("Value", v[:loc[2]]+n+v[loc[3]:])
				}
			}
		}
		return e
	}

	addTheirs := func(tid, rename string, after *xmltree.Node) {
		e := importTheirs(tid)
		newID := tid
		if rename != "" {
			newID = m.newTrackID()
		} else if n, ok := m.theirsRenumber[tid]; ok {
			newID = n
		}
		e.Set("Id", newID)
		if rename != "" {
			// EffectiveName is derived by Live; an empty UserName means auto-named.
			user, eff := e.Find("Name/UserName"), e.Find("Name/EffectiveName")
			name := user.Attr("Value")
			if name == "" {
				name = autoNumberRE.ReplaceAllString(eff.Attr("Value"), "")
			}
			user.Set("Value", name+rename)
			eff.Set("Value", name+rename)
		}
		if after != nil {
			// A conflict copy sits next to ours' track, in the same group/output.
			for _, p := range placementPaths {
				replaceChild(e, p, after.Find(p).Clone())
			}
		}
		m.setIdent(e, ident{"theirs": tid})
		mergedByID[newID] = e
		if after != nil {
			tracksElem.Insert(tracksElem.IndexOf(after)+1, e)
		} else {
			insertAfterTheirsNeighbour(tid, e)
		}
	}

	replaceWithTheirs := func(tid string) {
		old := mergedByID[tid]
		n := importTheirs(tid)
		replaceAt(tracksElem, old, n)
		m.setIdent(n, m.popIdent(old))
		mergedByID[tid] = n
		placed[tid] = n
	}

	remove := func(tid string) {
		e := mergedByID[tid]
		delete(mergedByID, tid)
		tracksElem.Remove(e)
		m.popIdent(e)
	}

	var allIDs []string
	seen := map[string]bool{}
	for _, side := range []string{"ours", "theirs", "base"} {
		for _, t := range m.sides[side].Tracks() {
			if !seen[t.ID()] {
				seen[t.ID()] = true
				allIDs = append(allIDs, t.ID())
			}
		}
	}

	for _, tid := range allIDs {
		b, o, t := m.track("base", tid), m.track("ours", tid), m.track("theirs", tid)
		decision := threeWay(m.trackFP(b), m.trackFP(o), m.trackFP(t))
		any := t
		if any == nil {
			any = o
		}
		if any == nil {
			any = b
		}
		label := fmt.Sprintf("%s \"%s\"", any.Kind(), any.Name())

		switch decision {
		case "ours":
			if o != nil && b != nil && m.trackFP(o) != m.trackFP(b) {
				m.log("%s: kept ours", label)
			}
			continue
		case "theirs":
			switch {
			case t == nil:
				remove(tid)
				m.log("%s: removed (deleted in theirs)", label)
			case o == nil:
				addTheirs(tid, "", nil)
				m.log("%s: added from theirs", label)
			default:
				replaceWithTheirs(tid)
				m.log("%s: took theirs", label)
			}
			continue
		}

		// conflict
		if b == nil {
			// Both sides created a track that happens to share an id.
			addTheirs(tid, "", nil)
			m.log("%s: added from theirs (id %s also used by ours, renumbered)", label, tid)
			continue
		}
		what := "modified on both sides"
		if o == nil {
			what = "deleted in ours, modified in theirs"
		} else if t == nil {
			what = "modified in ours, deleted in theirs"
		}
		key := "track:" + tid
		switch m.choice(key) {
		case "theirs":
			switch {
			case t == nil:
				remove(tid)
			case o == nil:
				addTheirs(tid, "", nil)
			default:
				replaceWithTheirs(tid)
			}
			m.conflict(key, label, what, "theirs")
		case "both":
			switch {
			case o != nil && t != nil:
				addTheirs(tid, " [theirs]", mergedByID[tid])
				m.conflict(key, label, what, "kept both (theirs added as a copy)")
			case o == nil:
				addTheirs(tid, "", nil)
				m.conflict(key, label, what, "restored theirs")
			default:
				m.conflict(key, label, what, "kept ours")
			}
		case "ours":
			m.conflict(key, label, what, "kept ours")
		default:
			m.conflict(key, label, what, "unresolved (kept ours)")
		}
	}
}

// mergePlacement merges group membership and output routing separately from
// the track body.
func (m *merger) mergePlacement() {
	for _, e := range append([]*xmltree.Node(nil), m.identOrder...) {
		id := m.idents[e]
		if _, ok := id["base"]; !ok {
			continue
		}
		if _, ok := id["ours"]; !ok {
			continue
		}
		if _, ok := id["theirs"]; !ok {
			continue
		}
		src := map[string]*xmltree.Node{}
		for _, side := range []string{"base", "ours", "theirs"} {
			src[side] = m.byID[side][id[side]].Elem
		}
		decision := threeWay(placementFP(src["base"]), placementFP(src["ours"]), placementFP(src["theirs"]))
		label := fmt.Sprintf("%s \"%s\"", e.Tag, als.Track{Elem: e}.Name())
		if decision == "conflict" {
			key := "placement:" + id["ours"]
			decision = m.resolve(key)
			m.conflict(key, label+" placement", "group/output changed on both sides", decision)
		} else if decision == "theirs" && placementFP(src["theirs"]) != placementFP(src["ours"]) {
			m.log("%s: took theirs' group/output", label)
		}
		donor := src[decision]
		if placementFP(donor) != placementFP(e) {
			for _, p := range placementPaths {
				replaceChild(e, p, donor.Find(p).Clone())
			}
			if decision == "theirs" {
				g := e.Child("TrackGroupId")
				if n, ok := m.theirsRenumber[g.Attr("Value")]; ok {
					g.Set("Value", n)
				}
			}
		}
	}
}

// fixReferences ungroups tracks whose group is gone and reports routing to
// removed tracks.
func (m *merger) fixReferences() {
	tracks := m.merged.Tracks()
	ids, groups := map[string]bool{}, map[string]bool{}
	for _, t := range tracks {
		ids[t.ID()] = true
		if t.Kind() == "GroupTrack" {
			groups[t.ID()] = true
		}
	}
	for _, t := range tracks {
		if g := t.Elem.Child("TrackGroupId"); g != nil && g.Attr("Value") != "-1" && !groups[g.Attr("Value")] {
			m.log("track \"%s\": group %s no longer exists, ungrouped", t.Name(), g.Attr("Value"))
			g.Set("Value", "-1")
			// Live routes former group members to the main output.
			if out := t.Elem.Find("DeviceChain/AudioOutputRouting"); out != nil && out.Val("Target", "") == "AudioOut/GroupTrack" {
				out.Child("Target").Set("Value", "AudioOut/Main")
				out.Child("UpperDisplayString").Set("Value", "Master")
			}
		}
		for _, target := range t.Elem.Iter("Target") {
			if mm := als.RoutingTrackRE.FindStringSubmatch(target.Attr("Value")); mm != nil && !ids[mm[1]] {
				m.log("track \"%s\": routing %s points to a removed track", t.Name(), target.Attr("Value"))
			}
		}
	}
}

// mergeOrder 3-way merges track order, then keeps groups contiguous and
// returns last.
func (m *merger) mergeOrder() {
	tracksElem := m.merged.TracksElem()
	current := append([]*xmltree.Node(nil), tracksElem.Children...)
	orders := map[string][]string{}
	for name, s := range m.sides {
		for _, t := range s.Tracks() {
			orders[name] = append(orders[name], t.ID())
		}
	}
	inAll := map[string]bool{}
	for _, tid := range orders["base"] {
		if contains(orders["ours"], tid) && contains(orders["theirs"], tid) {
			inAll[tid] = true
		}
	}
	filter := func(o []string) string {
		var out []string
		for _, tid := range o {
			if inAll[tid] {
				out = append(out, tid)
			}
		}
		return strings.Join(out, ",")
	}
	ob, oo, ot := filter(orders["base"]), filter(orders["ours"]), filter(orders["theirs"])
	decision := threeWay(ob, oo, ot)
	if decision == "conflict" {
		decision = m.resolve("order")
		m.conflict("order", "track order", "reordered on both sides", decision)
	}
	if decision == "theirs" && ot != oo {
		m.log("track order: took theirs")
		rank := map[string]int{}
		for i, tid := range orders["theirs"] {
			rank[tid] = i
		}
		var ordered []*xmltree.Node
		for _, e := range current {
			if _, ok := m.idents[e]["theirs"]; ok {
				ordered = append(ordered, e)
			}
		}
		sort.SliceStable(ordered, func(i, j int) bool {
			return rank[m.idents[ordered[i]]["theirs"]] < rank[m.idents[ordered[j]]["theirs"]]
		})
		// Ours-only tracks stay right after their previous neighbour in ours' layout.
		for i, e := range current {
			if nodeIndex(ordered, e) >= 0 {
				continue
			}
			at := 0
			for k := i - 1; k >= 0; k-- {
				if p := nodeIndex(ordered, current[k]); p >= 0 {
					at = p + 1
					break
				}
			}
			ordered = append(ordered, nil)
			copy(ordered[at+1:], ordered[at:])
			ordered[at] = e
		}
		current = ordered
	}

	// Group members follow their group track contiguously; returns go last.
	groups := map[string]bool{}
	for _, e := range current {
		if e.Tag == "GroupTrack" {
			groups[e.Attr("Id")] = true
		}
	}
	children := map[string][]*xmltree.Node{}
	var roots []*xmltree.Node
	for _, e := range current {
		if e.Tag == "ReturnTrack" {
			continue
		}
		if gid := (als.Track{Elem: e}).GroupID(); groups[gid] {
			children[gid] = append(children[gid], e)
		} else {
			roots = append(roots, e)
		}
	}
	var final []*xmltree.Node
	var emit func(e *xmltree.Node)
	emit = func(e *xmltree.Node) {
		final = append(final, e)
		if e.Tag == "GroupTrack" {
			for _, c := range children[e.Attr("Id")] {
				emit(c)
			}
		}
	}
	for _, e := range roots {
		emit(e)
	}
	for _, e := range current {
		if e.Tag == "ReturnTrack" {
			final = append(final, e)
		}
	}
	tracksElem.Children = final
	retail(tracksElem)
}

func (m *merger) rebuildSends() {
	var returns []als.Track
	for _, t := range m.merged.Tracks() {
		if t.Kind() == "ReturnTrack" {
			returns = append(returns, t)
		}
	}
	sideReturns := map[string][]string{}
	for name, s := range m.sides {
		for _, t := range s.Tracks() {
			if t.Kind() == "ReturnTrack" {
				sideReturns[name] = append(sideReturns[name], t.ID())
			}
		}
	}
	holder := func(side string, tid, rid ident) *xmltree.Node {
		t, okT := tid[side]
		r, okR := rid[side]
		if !okT || !okR {
			return nil
		}
		idx := indexOf(sideReturns[side], r)
		if idx < 0 {
			return nil
		}
		holders := m.byID[side][t].Elem.FindAll("DeviceChain/Mixer/Sends/TrackSendHolder")
		if idx < len(holders) {
			return holders[idx]
		}
		return nil
	}
	var template *xmltree.Node
	for _, s := range []*als.LiveSet{m.merged, m.base, m.ours, m.theirs} {
		if hs := s.Root.Iter("TrackSendHolder"); len(hs) > 0 {
			template = hs[0]
			break
		}
	}
	fp := func(h *xmltree.Node) string {
		if h == nil {
			return none
		}
		return als.Fingerprint(h)
	}

	for _, t := range m.merged.Tracks() {
		sends := t.Elem.Find("DeviceChain/Mixer/Sends")
		if sends == nil {
			continue
		}
		tident := m.identOf(t)
		var holders []*xmltree.Node
		for _, r := range returns {
			rident := m.identOf(r)
			hb, ho, ht := holder("base", tident, rident), holder("ours", tident, rident), holder("theirs", tident, rident)
			decision := threeWay(fp(hb), fp(ho), fp(ht))
			if decision == "conflict" {
				key := "send:" + t.ID() + ":" + r.ID()
				decision = m.resolve(key)
				m.conflict(key, fmt.Sprintf("send \"%s\" -> \"%s\"", t.Name(), r.Name()), "changed on both sides", decision)
			}
			chosen := ho
			if decision == "theirs" {
				chosen = ht
			}
			if chosen == nil && decision == "ours" && ho == nil {
				chosen = ht // track or return unknown to ours: theirs is the only source
			}
			var h *xmltree.Node
			switch {
			case chosen != nil && chosen == ht:
				h = m.importNode(chosen)
			case chosen != nil:
				h = chosen.Clone()
			default:
				h = m.importNode(template)
				send := h.Child("Send")
				send.Child("Manual").Set("Value", send.Val("MidiControllerRange/Min", ""))
			}
			holders = append(holders, h)
		}
		for i, h := range holders {
			h.Set("Id", strconv.Itoa(i))
		}
		sends.Children = holders
		retail(sends)
	}

	sendsPre := m.merged.LiveSet().Child("SendsPre")
	if sendsPre == nil {
		return
	}
	var values []string
	for _, r := range returns {
		rident := m.identOf(r)
		side := "theirs"
		if _, ok := rident["ours"]; ok {
			side = "ours"
		}
		idx := indexOf(sideReturns[side], rident[side])
		src := m.sides[side].LiveSet().Child("SendsPre")
		if idx < len(src.Children) {
			values = append(values, src.Children[idx].Attr("Value"))
		} else {
			values = append(values, "false")
		}
	}
	tmpl := &xmltree.Node{Tag: "SendPreBool"}
	if len(sendsPre.Children) > 0 {
		tmpl = sendsPre.Children[0]
	}
	var pre []*xmltree.Node
	for i, v := range values {
		e := tmpl.Clone()
		e.Set("Id", strconv.Itoa(i))
		e.Set("Value", v)
		pre = append(pre, e)
	}
	sendsPre.Children = pre
	retail(sendsPre)
}

func (m *merger) fitClipSlots() {
	scenes := m.merged.SceneCount()
	var empty, groupSlot *xmltree.Node
	for _, s := range []*als.LiveSet{m.merged, m.ours, m.theirs, m.base} {
		for _, cs := range s.Root.Iter("ClipSlot") {
			if v := cs.Find("ClipSlot/Value"); v != nil && len(v.Children) == 0 {
				empty = cs
				break
			}
		}
		if empty != nil {
			break
		}
	}
	for _, s := range []*als.LiveSet{m.merged, m.ours, m.theirs, m.base} {
		if gs := s.Root.Iter("GroupTrackSlot"); len(gs) > 0 {
			groupSlot = gs[0]
			break
		}
	}
	for _, t := range m.merged.Tracks() {
		if t.Kind() == "GroupTrack" {
			if slots := t.Elem.Child("Slots"); groupSlot != nil && slots != nil {
				for len(slots.Children) < scenes {
					slots.Append(groupSlot.Clone())
				}
				if len(slots.Children) > scenes {
					slots.Children = slots.Children[:scenes]
				}
				for i, gs := range slots.Children {
					gs.Set("Id", strconv.Itoa(i))
				}
				retail(slots)
			}
		}
		if t.Kind() == "ReturnTrack" {
			continue
		}
		for _, seq := range []string{"MainSequencer", "FreezeSequencer"} {
			slots := t.Elem.Find("DeviceChain/" + seq + "/ClipSlotList")
			if slots == nil || len(slots.Children) == scenes {
				continue
			}
			for len(slots.Children) < scenes && empty != nil {
				slots.Append(empty.Clone())
			}
			for len(slots.Children) > scenes && len(slots.Children[len(slots.Children)-1].Find("ClipSlot/Value").Children) == 0 {
				slots.Children = slots.Children[:len(slots.Children)-1]
			}
			if len(slots.Children) != scenes {
				m.log("track \"%s\": has clips in scenes that no longer exist", t.Name())
			}
			for i, cs := range slots.Children {
				cs.Set("Id", strconv.Itoa(i))
			}
			retail(slots)
		}
	}
}

func (m *merger) renumberPointees() {
	all := m.merged.PointeeElements()
	used := map[string]bool{}
	nextID := max(m.ours.NextPointeeID()-1, m.theirs.NextPointeeID()-1)
	for _, e := range all {
		if !m.imported[e] {
			used[e.Attr("Id")] = true
		}
		n, _ := strconv.Atoi(e.Attr("Id"))
		nextID = max(nextID, n)
	}
	nextID++

	unitOf := map[*xmltree.Node]*xmltree.Node{}
	for _, u := range m.merged.Units() {
		u.Walk(func(e *xmltree.Node) bool { unitOf[e] = u; return true })
	}

	remaps := map[*xmltree.Node]map[string]string{}
	var remapOrder []*xmltree.Node
	renumbered := 0
	for _, e := range all {
		if !m.imported[e] {
			continue
		}
		old := e.Attr("Id")
		if used[old] {
			n := strconv.Itoa(nextID)
			nextID++
			e.Set("Id", n)
			renumbered++
			if u := unitOf[e]; u != nil {
				if remaps[u] == nil {
					remaps[u] = map[string]string{}
					remapOrder = append(remapOrder, u)
				}
				remaps[u][old] = n
			}
			old = n
		}
		used[old] = true
	}
	for _, u := range remapOrder {
		mapping := remaps[u]
		kept := map[string]bool{}
		u.Walk(func(e *xmltree.Node) bool {
			if e.Has("Id") && als.IsPointeeTag(e.Tag) && !m.imported[e] {
				kept[e.Attr("Id")] = true
			}
			return true
		})
		for _, ref := range u.Iter("PointeeId") {
			if n, ok := mapping[ref.Attr("Value")]; ok && !kept[ref.Attr("Value")] {
				ref.Set("Value", n)
			}
		}
	}
	if renumbered > 0 {
		m.log("renumbered %d colliding automation/modulation target id(s)", renumbered)
	}
	m.merged.SetNextPointeeID(nextID)
}

func (m *merger) dropDanglingEnvelopes() {
	for _, u := range m.merged.Units() {
		local := als.LocalPointeeIDs(u)
		for _, parent := range u.Iter("") {
			for _, env := range append([]*xmltree.Node(nil), parent.Children...) {
				ref := env.Find("EnvelopeTarget/PointeeId")
				if ref != nil && !local[ref.Attr("Value")] {
					parent.Remove(env)
					retail(parent)
					m.log("%s %s: dropped automation for a removed parameter", u.Tag, u.Attr("Id"))
				}
			}
		}
	}
}

func (m *merger) run() {
	m.mergeGlobals()
	m.mergeTracks()
	m.mergePlacement()
	m.fixReferences()
	m.mergeOrder()
	m.rebuildSends()
	m.fitClipSlots()
	m.renumberPointees()
	m.dropDanglingEnvelopes()
	m.result.Issues = als.Validate(m.merged)
}

// --- helpers ---

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

func contains(xs []string, x string) bool { return indexOf(xs, x) >= 0 }

func nodeIndex(xs []*xmltree.Node, x *xmltree.Node) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}
