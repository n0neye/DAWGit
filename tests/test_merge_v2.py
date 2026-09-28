"""Merges against SampleAbletonProject_v2: group track, session clips,
automation and an added audio effect, all made in Live."""

import copy
import unittest

from dawgit.diff import _envelopes
from dawgit.merge import TRACK_SKIP, merge_sets
from dawgit.normalize import fingerprint

from tests import helpers as h
from tests.helpers import CURRENT, CURRENT_V2, backup_v2, load


def order(s):
    return [t.id for t in s.tracks()]


def names(s):
    return [t.name for t in s.tracks()]


def content_by_name(s):
    """Track content keyed by name, with group given by name instead of id."""
    by_id = s.track_by_id()
    out = {}
    for t in s.tracks():
        group = by_id[t.group_id].name if t.group_id in by_id else None
        elem = copy.deepcopy(t.elem)
        del elem.attrib["Id"]
        out[t.name] = (group, fingerprint(elem, TRACK_SKIP))
    return out


def volume(s, tid):
    return h.track(s, tid).elem.find("DeviceChain/Mixer/Volume/Manual").get("Value")


class MergeV2Test(unittest.TestCase):
    def assertClean(self, r):
        self.assertEqual(r.issues, [], r.report())

    def ours_v1_edits(self):
        """Edits on v1 that do not touch anything v2 changed."""
        ours = load(CURRENT)
        h.set_volume(ours, "2", 0.4)  # return A
        h.set_tempo(ours, 120.0)
        h.duplicate_track(ours, "8", "Ours Drums 2")
        return ours

    def test_v2_changes_combine_with_unrelated_edits(self):
        base, theirs = load(CURRENT), load(CURRENT_V2)
        r = merge_sets(base, self.ours_v1_edits(), theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        m = r.merged
        # Group with its members right after it; ours' new track after drums.
        # (Ours' new track and theirs' group were both created as id 15.)
        self.assertEqual(names(m), ["MIDI Tracks", "2-Basic Saturated Bass", "3-Vital", "4-80s Beat 90 bpm",
                                    "Ours Drums 2", "5 Bounce + Reverb", "A-Reverb", "B-Delay"])
        group = next(t for t in m.tracks() if t.kind == "GroupTrack")
        self.assertEqual(h.track(m, "12").group_id, group.id)
        # Ours' edits.
        self.assertEqual(m.tempo, "120.0")
        self.assertEqual(volume(m, "2"), "0.4")
        # Theirs' session clips, effect and automation.
        self.assertEqual(len([c for c in h.track(m, "8").clips() if c.location.startswith("session")]), 2)
        self.assertEqual(h.track(m, "14").device_names(), ["Reverb"])
        self.assertIn("Mixer: Volume", _envelopes(h.track(m, "13").elem))
        self.assertIn("Reverb: MixDirect", _envelopes(h.track(m, "14").elem))

    def test_merge_direction_does_not_change_tracks(self):
        base, theirs = load(CURRENT), load(CURRENT_V2)
        a = merge_sets(base, self.ours_v1_edits(), theirs).merged
        b = merge_sets(base, theirs, self.ours_v1_edits()).merged
        # Which side gets renumbered differs, so compare by name.
        self.assertEqual(names(a), names(b))
        self.assertEqual(content_by_name(a), content_by_name(b))
        self.assertEqual(a.tempo, b.tempo)

    def test_grouping_does_not_conflict_with_track_edit(self):
        # Theirs only put tracks 12/13 into a group; ours changed track 12's volume.
        base = load(CURRENT)
        ours = h.clone(base)
        h.set_volume(ours, "12", 0.3)
        theirs = load(backup_v2("204139"))
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        self.assertEqual(volume(r.merged, "12"), "0.3")
        self.assertEqual(h.track(r.merged, "12").group_id, "15")
        self.assertEqual(
            h.track(r.merged, "12").elem.find("DeviceChain/AudioOutputRouting/Target").get("Value"),
            "AudioOut/GroupTrack")

    def test_scene_added_pads_group_slots(self):
        base = load(backup_v2("204139"))
        ours, theirs = h.clone(base), h.clone(base)
        h.add_scene(ours)
        h.set_volume(theirs, "15", 0.5)
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(len(h.track(r.merged, "15").elem.find("Slots")), 9)

    def test_conflict_copy_stays_in_group(self):
        base = load(CURRENT_V2)
        ours, theirs = h.clone(base), h.clone(base)
        h.set_volume(ours, "13", 0.2)
        h.set_volume(theirs, "13", 0.9)
        r = merge_sets(base, ours, theirs, strategy="both")
        self.assertClean(r)
        copy_track = next(t for t in r.merged.tracks() if t.name.endswith("[theirs]"))
        self.assertEqual(copy_track.group_id, "15")
        self.assertEqual(order(r.merged)[:4], ["15", "12", "13", copy_track.id])
        # The copy's automation got fresh target ids and still resolves.
        self.assertIn("Mixer: Volume", _envelopes(copy_track.elem))

    def test_group_deleted_in_theirs_ungroups_ours_new_member(self):
        base = load(backup_v2("204139"))
        ours, theirs = h.clone(base), h.clone(base)
        new_id = h.duplicate_track(ours, "13", "Ours Vital 2")  # also inside group 15
        for tid in ("15",):
            h.delete_track(theirs, tid)
        for tid in ("12", "13"):
            t = h.track(theirs, tid).elem
            t.find("TrackGroupId").set("Value", "-1")
            t.find("DeviceChain/AudioOutputRouting/Target").set("Value", "AudioOut/Main")
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertNotIn("15", r.merged.track_by_id())
        self.assertEqual(h.track(r.merged, new_id).group_id, "-1")

    def test_automation_survives_both_side_renumbering(self):
        base = load(CURRENT_V2)
        ours, theirs = h.clone(base), h.clone(base)
        h.duplicate_track(ours, "14", "Ours Bounce 2")
        h.duplicate_track(theirs, "13", "Theirs Vital 2")
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        m = r.merged
        for t in m.tracks():
            for label in _envelopes(t.elem):
                self.assertFalse(label.startswith("target "), f"{t.name}: unresolved {label}")
        names = {t.name: t for t in m.tracks()}
        self.assertIn("Reverb: MixDirect", _envelopes(names["Ours Bounce 2"].elem))
        self.assertIn("Mixer: Volume", _envelopes(names["Theirs Vital 2"].elem))
        self.assertEqual(fingerprint(h.track(m, "14").elem), fingerprint(h.track(base, "14").elem))


if __name__ == "__main__":
    unittest.main()
