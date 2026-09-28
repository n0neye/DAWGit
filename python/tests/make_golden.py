"""Write golden outputs of the Python reference implementation for the Go port.

    cd python && python -m tests.make_golden

Creates ../testdata/golden (git-ignored):
  merge/<case>/{base,ours,theirs}.als, strategy.txt, expected.xml, report.txt
  diff.json    [{"a": ..., "b": ..., "expected": ...}]   (paths relative to repo root)
  validate.json [{"file": ..., "issues": [...]}]
"""

import gzip
import json
import shutil

from dawgit.diff import diff_sets
from dawgit.merge import merge_sets
from dawgit.validate import validate

from tests import helpers as h
from tests.helpers import CURRENT, CURRENT_V2, PROJECT, ROOT, load
from tests.make_live_samples import scenarios

OUT = ROOT / "testdata" / "golden"


def extra_scenarios():
    base = load(CURRENT)

    ours, theirs = h.clone(base), h.clone(base)
    h.delete_track(theirs, "14")
    yield "delete-theirs", base, ours, theirs, "fail"

    ours, theirs = h.clone(base), h.clone(base)
    h.set_volume(ours, "14", 0.1)
    h.delete_track(theirs, "14")
    for s in ("fail", "theirs", "both"):
        yield f"modify-vs-delete-{s}", base, ours, theirs, s

    ours, theirs = h.clone(base), h.clone(base)
    h.set_volume(ours, "12", 0.3)
    send = h.track(theirs, "12").elem.find("DeviceChain/Mixer/Sends/TrackSendHolder/Send/Manual")
    send.set("Value", "0.5")
    yield "send-vs-volume", base, ours, theirs, "fail"

    ours, theirs = h.clone(base), h.clone(base)
    h.set_volume(ours, "12", 0.3)
    h.set_volume(theirs, "12", 0.7)
    for s in ("fail", "ours", "theirs"):
        yield f"same-track-{s}", base, ours, theirs, s

    v2 = load(CURRENT_V2)
    split_a, split_b = load(PROJECT / "Split-A.als"), load(PROJECT / "Split-B.als")
    for s in ("fail", "ours", "theirs", "both"):
        yield f"real-split-{s}", v2, split_a, split_b, s
        yield f"real-split-reversed-{s}", v2, split_b, split_a, s


def write_set(s, path):
    s.save(path)


def main():
    if OUT.exists():
        shutil.rmtree(OUT)
    (OUT / "merge").mkdir(parents=True)

    n = 0
    for name, base, ours, theirs, strategy in list(scenarios()) + list(extra_scenarios()):
        d = OUT / "merge" / name
        d.mkdir()
        write_set(base, d / "base.als")
        write_set(ours, d / "ours.als")
        write_set(theirs, d / "theirs.als")
        (d / "strategy.txt").write_text(strategy, encoding="utf-8")
        r = merge_sets(load(d / "base.als"), load(d / "ours.als"), load(d / "theirs.als"), strategy=strategy)
        with gzip.open(d / "expected.xml.gz", "wb") as f:
            f.write(r.merged.to_xml_bytes())
        (d / "report.txt").write_text(r.report(), encoding="utf-8")
        n += 1

    sets = sorted(p for p in PROJECT.rglob("*.als") if not p.name.startswith("MergeTest-"))
    rel = lambda p: p.relative_to(ROOT).as_posix()
    diffs = []
    for a in sets:
        for b in sets:
            if a != b and (a.parent == b.parent or a.stem.split(" [")[0] == b.stem.split(" [")[0]):
                diffs.append({"a": rel(a), "b": rel(b), "expected": diff_sets(load(a), load(b)).render()})
    (OUT / "diff.json").write_text(json.dumps(diffs, ensure_ascii=False, indent=1), encoding="utf-8")
    (OUT / "validate.json").write_text(json.dumps(
        [{"file": rel(p), "issues": validate(load(p))} for p in sets], ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"{n} merge cases, {len(diffs)} diff pairs, {len(sets)} sets -> {OUT}")


if __name__ == "__main__":
    main()
