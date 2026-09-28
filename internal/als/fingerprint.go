package als

import (
	"crypto/sha1"
	"encoding/hex"
	"hash"
	"sort"
	"strings"

	"dawgit/internal/xmltree"
)

// NoiseElements are subtrees Live rewrites on save without a musical change:
// serialization counters, LOM handles, playhead, selection and view state.
var NoiseElements = map[string]bool{
	"LomId": true, "LomIdView": true, "IsContentSelectedInDocument": true,
	"ViewData": true, "ViewStates": true, "CurrentTime": true,
	"HighlightedTrackIndex": true, "TimeSelection": true,
	"ClipEnvelopeChooserViewState": true, "LastSelectedTimeableIndex": true,
	"LastSelectedClipEnvelopeIndex": true, "ScrollerTimePreserver": true,
	"SessionScrollPos": true, "SequencerNavigator": true, "SelectedDevice": true,
	"SelectedEnvelope": true, "IsExpanded": true, "BreakoutIsExpanded": true,
	"IsFolded": true, "ViewStateSessionTrackWidth": true, "WinPosX": true,
	"WinPosY": true, "OverwriteProtectionNumber": true,
	"IsArmed":          true, // record arm is per-session operator state
	"SavedPlayingSlot": true, // which session clip was playing at save time
	"EffectiveName":    true, // derived by Live from UserName, devices and track position
	"TrackUnfolded":    true, // group fold state
	"TakeId":           true, // lazily assigned (-1 -> 0) when Live reopens a set
}

// NoiseAttrs are attributes that are view state.
var NoiseAttrs = map[string]bool{
	"SelectedToolPanel": true, "SelectedTransformationName": true, "SelectedGeneratorName": true,
}

// Only track ids carry identity we care about; every other Id is a positional
// index, a serialization counter, or a pointee id (identity without meaning).
func keepID(tag string) bool { return TrackTags[tag] }

func feed(e *xmltree.Node, h hash.Hash, skip map[string]bool) {
	if NoiseElements[e.Tag] || skip[e.Tag] {
		return
	}
	h.Write([]byte("<" + e.Tag))
	names := make([]string, 0, len(e.Attrs))
	for _, a := range e.Attrs {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	for _, k := range names {
		if NoiseAttrs[k] || (k == "Id" && !keepID(e.Tag)) {
			continue
		}
		h.Write([]byte(" " + k + "=" + e.Attr(k)))
	}
	h.Write([]byte(">"))
	if text := strings.TrimSpace(e.Text); text != "" {
		h.Write([]byte(text))
	}
	for _, c := range e.Children {
		feed(c, h, skip)
	}
	h.Write([]byte("</>"))
}

// Fingerprint hashes e's musical content, ignoring noise and any subtree whose
// tag is in skip. A nil element hashes to "<none>".
func Fingerprint(e *xmltree.Node, skip ...string) string {
	if e == nil {
		return "<none>"
	}
	var sk map[string]bool
	if len(skip) > 0 {
		sk = make(map[string]bool, len(skip))
		for _, s := range skip {
			sk[s] = true
		}
	}
	h := sha1.New()
	feed(e, h, sk)
	return hex.EncodeToString(h.Sum(nil))
}
