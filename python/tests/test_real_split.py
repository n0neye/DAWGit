"""Merge of two divergent copies of v2, both edited by hand in Live.

Split-A: new group "Audios" (id 16) containing tracks 8 and 14; volume and
         reverb automation on track 14.
Split-B: tracks 8 and 14 switched off; Amp and volume automation on track 14;
         new MIDI track "Drum" (also id 16) with a 909 kit.
"""

import unittest

from dawgit.diff import _envelopes
from dawgit.merge import merge_sets

from tests.helpers import CURRENT_V2, PROJECT, load

SPLIT_A = PROJECT / "Split-A.als"
SPLIT_B = PROJECT / "Split-B.als"


def by_name(s):
    return {t.name: t for t in s.tracks()}


def speaker(t):
    return t.elem.find("DeviceChain/Mixer/Speaker/Manual").get("Value")


class RealSplitTest(unittest.TestCase):
    def merge(self, strategy):
        r = merge_sets(load(CURRENT_V2), load(SPLIT_A), load(SPLIT_B), strategy=strategy)
        self.assertEqual(r.issues, [], r.report())
        return r

    def test_only_the_track_both_edited_conflicts(self):
        r = self.merge("fail")
        self.assertEqual([c.unit for c in r.conflicts], ['AudioTrack "5 Bounce + Reverb"'])

    def test_non_conflicting_edits_combine(self):
        m = self.merge("ours").merged
        tracks = by_name(m)
        audios = tracks["Audios"]
        drum = tracks["Drum"]
        self.assertEqual(audios.kind, "GroupTrack")
        self.assertNotEqual(drum.id, audios.id)  # both were created as id 16
        beat = tracks["4-80s Beat 90 bpm"]
        self.assertEqual(beat.group_id, audios.id)  # A's grouping
        self.assertEqual(speaker(beat), "false")  # B's mute
        self.assertEqual(drum.group_id, "-1")
        self.assertEqual(drum.device_names(), ['DrumGroupDevice "909 Core Kit"'])

    def test_keep_both_puts_copy_in_ours_group(self):
        m = self.merge("both").merged
        tracks = by_name(m)
        ours, theirs = tracks["6 Bounce + Reverb"], tracks["# Bounce + Reverb [theirs]"]
        self.assertEqual(ours.group_id, tracks["Audios"].id)
        self.assertEqual(theirs.group_id, tracks["Audios"].id)
        self.assertEqual(theirs.device_names(), ["Amp", "Reverb"])
        self.assertEqual(sorted(_envelopes(ours.elem)), ["Mixer: Volume", "Reverb: MixDirect"])
        self.assertIn("Mixer: Volume", _envelopes(theirs.elem))
        order = [t.name for t in m.tracks()]
        self.assertEqual(order.index("# Bounce + Reverb [theirs]"), order.index("6 Bounce + Reverb") + 1)


if __name__ == "__main__":
    unittest.main()
