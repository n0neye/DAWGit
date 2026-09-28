package als

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"dawgit/internal/xmltree"
)

// RoutingTrackRE finds track references in routing targets ("AudioIn/Track.13/PostFX").
var RoutingTrackRE = regexp.MustCompile(`Track\.(\d+)`)

// Units are the top-level containers that own their pointee ids and the
// envelope references to them.
func (s *LiveSet) Units() []*xmltree.Node {
	var out []*xmltree.Node
	for _, t := range s.Tracks() {
		out = append(out, t.Elem)
	}
	for _, tag := range []string{"MainTrack", "PreHearTrack"} {
		if e := s.LiveSet().Child(tag); e != nil {
			out = append(out, e)
		}
	}
	return out
}

// LocalPointeeIDs returns the pointee ids defined inside unit.
func LocalPointeeIDs(unit *xmltree.Node) map[string]bool {
	ids := map[string]bool{}
	unit.Walk(func(e *xmltree.Node) bool {
		if e.Has("Id") && IsPointeeTag(e.Tag) {
			ids[e.Attr("Id")] = true
		}
		return true
	})
	return ids
}

// Validate checks the structural invariants Live relies on. Messages match
// the Python reference implementation.
func Validate(s *LiveSet) []string {
	var issues []string
	add := func(format string, a ...any) { issues = append(issues, fmt.Sprintf(format, a...)) }

	counts := map[int]int{}
	var order []int
	maxID := -1
	for _, e := range s.PointeeElements() {
		id, _ := strconv.Atoi(e.Attr("Id"))
		if counts[id] == 0 {
			order = append(order, id)
		}
		counts[id]++
		maxID = max(maxID, id)
	}
	var dups []int
	for _, id := range order {
		if counts[id] > 1 {
			dups = append(dups, id)
		}
	}
	if len(dups) > 0 {
		sort.Ints(dups)
		more := ""
		if len(dups) > 10 {
			more = " ..."
		}
		add("duplicate pointee ids: %s%s", ReprInts(dups[:min(10, len(dups))]), more)
	}
	if maxID >= 0 && s.NextPointeeID() <= maxID {
		add("NextPointeeId %d <= max pointee id %d", s.NextPointeeID(), maxID)
	}

	for _, unit := range s.Units() {
		local := LocalPointeeIDs(unit)
		for _, ref := range unit.Iter("PointeeId") {
			if !local[ref.Attr("Value")] {
				add("%s %s: dangling PointeeId %s", unit.Tag, unit.Attr("Id"), ref.Attr("Value"))
			}
		}
	}

	tracks := s.Tracks()
	trackIDs := map[string]bool{}
	trackCount := map[string]int{}
	var trackOrder []string
	for _, t := range tracks {
		trackIDs[t.ID()] = true
		if trackCount[t.ID()] == 0 {
			trackOrder = append(trackOrder, t.ID())
		}
		trackCount[t.ID()]++
	}
	var dupTracks []string
	for _, id := range trackOrder {
		if trackCount[id] > 1 {
			dupTracks = append(dupTracks, id)
		}
	}
	if len(dupTracks) > 0 {
		add("duplicate track ids: %s", ReprStrings(dupTracks))
	}

	seenReturn := false
	for _, t := range tracks {
		if t.Kind() == "ReturnTrack" {
			seenReturn = true
		} else if seenReturn {
			add("return tracks must come after all other tracks")
			break
		}
	}

	// Group members must directly follow their group track (nested groups inside).
	var open []string
	for _, t := range tracks {
		if t.Kind() == "ReturnTrack" {
			break
		}
		for len(open) > 0 && open[len(open)-1] != t.GroupID() {
			open = open[:len(open)-1]
		}
		if t.GroupID() != "-1" && (len(open) == 0 || open[len(open)-1] != t.GroupID()) {
			add("track %s: not placed inside its group %s", t.ID(), t.GroupID())
		}
		if t.Kind() == "GroupTrack" {
			open = append(open, t.ID())
		}
	}

	scenes := s.SceneCount()
	nReturns := 0
	groupIDs := map[string]bool{}
	for _, t := range tracks {
		switch t.Kind() {
		case "ReturnTrack":
			nReturns++
		case "GroupTrack":
			groupIDs[t.ID()] = true
		}
	}
	for _, t := range tracks {
		if t.Kind() != "ReturnTrack" { // return tracks have no clip slots
			for _, seq := range []string{"MainSequencer", "FreezeSequencer"} {
				if slots := t.Elem.Find("DeviceChain/" + seq + "/ClipSlotList"); slots != nil && len(slots.Children) != scenes {
					add("track %s %s: %d clip slots, %d scenes", t.ID(), seq, len(slots.Children), scenes)
				}
			}
		}
		if t.Kind() == "GroupTrack" {
			if gs := t.Elem.Child("Slots"); gs != nil && len(gs.Children) != scenes {
				add("group %s: %d slots, %d scenes", t.ID(), len(gs.Children), scenes)
			}
		}
		if n := len(t.Elem.FindAll("DeviceChain/Mixer/Sends/TrackSendHolder")); n != nReturns {
			add("track %s: %d sends, %d return tracks", t.ID(), n, nReturns)
		}
		if t.GroupID() != "-1" && !groupIDs[t.GroupID()] {
			add("track %s: TrackGroupId %s does not exist", t.ID(), t.GroupID())
		}
		for _, target := range t.Elem.Iter("Target") {
			if m := RoutingTrackRE.FindStringSubmatch(target.Attr("Value")); m != nil && !trackIDs[m[1]] {
				add("track %s: routing to missing track %s", t.ID(), target.Attr("Value"))
			}
		}
	}

	if sp := s.LiveSet().Child("SendsPre"); sp != nil && len(sp.Children) != nReturns {
		add("SendsPre has %d entries, %d return tracks", len(sp.Children), nReturns)
	}
	return issues
}

// ReprStrings formats like Python's repr of a list of str: ['a', 'b'].
func ReprStrings(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = "'" + x + "'"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// ReprInts formats like Python's repr of a list of int: [1, 2].
func ReprInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
