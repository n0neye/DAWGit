import unittest

from dawgit.als import LiveSet
from dawgit.diff import diff_sets
from dawgit.merge import merge_sets
from dawgit.normalize import fingerprint

from tests import helpers as h
from tests.helpers import CURRENT, backup, load


def names(s: LiveSet) -> list[str]:
    return [t.name for t in s.tracks()]


class MergeTest(unittest.TestCase):
    def assertClean(self, result):
        self.assertEqual(result.issues, [], result.report())
        # Survives a save/load cycle.
        again = LiveSet.from_xml_bytes(result.merged.to_xml_bytes())
        self.assertEqual(len(again.tracks()), len(result.merged.tracks()))

    def test_identical_sides_is_noop(self):
        base = load(CURRENT)
        r = merge_sets(base, h.clone(base), h.clone(base))
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        self.assertTrue(diff_sets(base, r.merged).empty)

    def test_fast_forward_equals_theirs(self):
        # Ours has no changes since base: result is exactly theirs.
        base, theirs = load(backup("200019")), load(backup("200348"))
        ours = h.clone(base)
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        self.assertTrue(diff_sets(theirs, r.merged).empty, diff_sets(theirs, r.merged).render())

    def test_edits_on_different_tracks_combine(self):
        base = load(backup("200025"))
        ours = load(backup("200235"))  # Vital gets a clip, bass track switched off
        theirs = h.clone(base)
        h.set_volume(theirs, "8", 0.5)
        h.set_tempo(theirs, 128.0)
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        m = r.merged
        self.assertEqual(m.tempo, "128.0")
        self.assertEqual(h.track(m, "8").elem.find("DeviceChain/Mixer/Volume/Manual").get("Value"), "0.5")
        self.assertEqual(len(h.track(m, "13").clips()), 1)
        self.assertEqual(h.track(m, "12").elem.find("DeviceChain/Mixer/Speaker/Manual").get("Value"), "false")

    def test_both_sides_add_tracks_with_colliding_ids(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        ours_id = h.duplicate_track(ours, "12", "Ours Bass 2")
        theirs_id = h.duplicate_track(theirs, "13", "Theirs Vital 2")
        self.assertEqual(ours_id, theirs_id)  # same id, different tracks
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        self.assertIn("Ours Bass 2", names(r.merged))
        self.assertIn("Theirs Vital 2", names(r.merged))
        # Theirs' track sits after its neighbour from theirs' order.
        n = names(r.merged)
        self.assertEqual(n.index("Theirs Vital 2"), n.index("2-Vital") + 1)
        # Automation/modulation ids of the two new tracks were deconflicted.
        self.assertGreater(r.merged.next_pointee_id, max(ours.next_pointee_id, theirs.next_pointee_id))

    def test_same_track_conflict(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        h.set_volume(ours, "12", 0.3)
        h.set_volume(theirs, "12", 0.7)

        r = merge_sets(base, ours, theirs, strategy="fail")
        self.assertEqual(len(r.conflicts), 1)
        self.assertIn("modified on both sides", r.conflicts[0].description)

        r = merge_sets(base, ours, theirs, strategy="theirs")
        self.assertClean(r)
        self.assertEqual(h.track(r.merged, "12").elem.find("DeviceChain/Mixer/Volume/Manual").get("Value"), "0.7")

        r = merge_sets(base, ours, theirs, strategy="both")
        self.assertClean(r)
        n = names(r.merged)
        self.assertIn("1-Basic Saturated Bass [theirs]", n)
        self.assertEqual(n.index("1-Basic Saturated Bass [theirs]"), n.index("1-Basic Saturated Bass") + 1)

    def test_theirs_adds_return_track(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        h.set_volume(ours, "13", 0.25)  # unrelated edit on ours
        h.add_return(theirs, "C-Chorus")
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        self.assertEqual([t.name for t in r.merged.tracks() if t.kind == "ReturnTrack"],
                         ["A-Reverb", "B-Delay", "C-Chorus"])
        self.assertEqual(h.track(r.merged, "13").elem.find("DeviceChain/Mixer/Volume/Manual").get("Value"), "0.25")

    def test_both_add_return_tracks(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        h.add_return(ours, "C-Ours")
        h.add_return(theirs, "C-Theirs")
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual([t.name for t in r.merged.tracks() if t.kind == "ReturnTrack"],
                         ["A-Reverb", "B-Delay", "C-Ours", "C-Theirs"])

    def test_send_edits_merge_independently_of_track_body(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        h.set_volume(ours, "12", 0.3)
        send = h.track(theirs, "12").elem.find("DeviceChain/Mixer/Sends/TrackSendHolder/Send/Manual")
        send.set("Value", "0.5")
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        t = h.track(r.merged, "12").elem
        self.assertEqual(t.find("DeviceChain/Mixer/Volume/Manual").get("Value"), "0.3")
        self.assertEqual(t.find("DeviceChain/Mixer/Sends/TrackSendHolder/Send/Manual").get("Value"), "0.5")

    def test_theirs_adds_scene(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        h.duplicate_track(ours, "8", "Ours Drums 2")  # ours' new track has 8 slots
        h.add_scene(theirs)
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.merged.scene_count(), 9)

    def test_scene_added_does_not_conflict_with_track_edits(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        h.set_volume(ours, "12", 0.3)
        h.add_scene(theirs)
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertEqual(r.conflicts, [])
        self.assertEqual(h.track(r.merged, "12").elem.find("DeviceChain/Mixer/Volume/Manual").get("Value"), "0.3")

    def test_delete_vs_unchanged_and_delete_vs_modify(self):
        base = load(CURRENT)
        ours, theirs = h.clone(base), h.clone(base)
        h.delete_track(theirs, "14")
        r = merge_sets(base, ours, theirs)
        self.assertClean(r)
        self.assertNotIn("14", r.merged.track_by_id())

        h.set_volume(ours, "14", 0.1)
        r = merge_sets(base, ours, theirs, strategy="fail")
        self.assertIn("modified in ours, deleted in theirs", r.conflicts[0].description)

    def test_merge_is_symmetric_in_content(self):
        base = load(backup("200025"))
        ours = load(backup("200235"))
        theirs = h.clone(base)
        h.set_volume(theirs, "8", 0.5)
        a = merge_sets(base, ours, theirs).merged
        b = merge_sets(base, theirs, ours).merged
        for tid in a.track_by_id():
            self.assertEqual(fingerprint(h.track(a, tid).elem), fingerprint(h.track(b, tid).elem))


if __name__ == "__main__":
    unittest.main()
