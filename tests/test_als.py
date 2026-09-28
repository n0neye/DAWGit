import glob
import gzip
import unittest

from dawgit.als import LiveSet
from dawgit.diff import diff_sets
from dawgit.validate import validate

from tests.helpers import CURRENT, PROJECT, backup, load

ALL_SETS = sorted(glob.glob(str(PROJECT / "**" / "*.als"), recursive=True))


class RoundTripTest(unittest.TestCase):
    def test_save_is_byte_exact(self):
        for path in ALL_SETS:
            with self.subTest(path=path):
                with gzip.open(path) as f:
                    raw = f.read()
                self.assertEqual(LiveSet.load(path).to_xml_bytes(), raw)

    def test_fixtures_are_valid(self):
        for path in ALL_SETS:
            with self.subTest(path=path):
                self.assertEqual(validate(LiveSet.load(path)), [])


class ModelTest(unittest.TestCase):
    def test_tracks_devices_clips(self):
        s = load(CURRENT)
        self.assertEqual(s.tempo, "111")
        tracks = {t.id: t for t in s.tracks()}
        self.assertEqual([t.kind for t in s.tracks()],
                         ["MidiTrack", "MidiTrack", "AudioTrack", "AudioTrack", "ReturnTrack", "ReturnTrack"])
        self.assertEqual(tracks["13"].device_names(), ["Vital (Vst3)"])
        self.assertEqual([c.name for c in tracks["8"].clips()], ["80s Beat 90 bpm"])
        self.assertEqual(s.plugins(), ["Vital (Vst3)"])

    def test_sample_refs(self):
        refs = {r.relative_path: r for r in load(CURRENT).sample_refs()}
        self.assertIn("Samples/Processed/Bounce/Bounce 2-Analog [2026-09-28 195449]-2.wav", refs)
        self.assertEqual(refs["Samples/Loops/Drums/Full/80s Beat 90 bpm.wav"].pack, "Core Library")


class DiffTest(unittest.TestCase):
    def test_noise_only_saves_have_no_diff(self):
        # Saved again with only view/selection/playhead changes.
        self.assertTrue(diff_sets(load(backup("200235")), load(backup("200308"))).empty)
        self.assertTrue(diff_sets(load(backup("200308")), load(backup("200345"))).empty)

    def test_detects_musical_changes(self):
        text = diff_sets(load(backup("195411")), load(backup("195505"))).render()
        self.assertIn("tempo: 120 -> 111", text)
        self.assertIn("+ device UltraAnalog", text)
        self.assertIn('+ clip "Arp Bells C Minor 121 bpm"', text)

    def test_volume_in_db(self):
        text = diff_sets(load(backup("200348")), load(CURRENT)).render()
        self.assertIn("mixer volume: +0.0 dB -> -5.0 dB", text)


if __name__ == "__main__":
    unittest.main()
