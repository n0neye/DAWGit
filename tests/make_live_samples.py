"""Write merge results into the sample project so they can be opened in Live.

    python -m tests.make_live_samples

Outputs MergeTest-*.als next to SampleAbletonProject.als (git-ignored).
"""

from dawgit.merge import merge_sets

from tests import helpers as h
from tests.helpers import CURRENT, PROJECT, backup, load


def scenarios():
    base = load(CURRENT)

    ours, theirs = h.clone(base), h.clone(base)
    h.duplicate_track(ours, "12", "Ours Bass 2")
    h.duplicate_track(theirs, "13", "Theirs Vital 2")
    yield "1-both-add-tracks", base, ours, theirs, "fail"

    ours, theirs = h.clone(base), h.clone(base)
    h.set_volume(ours, "12", 0.3)
    h.set_volume(theirs, "12", 0.7)
    h.rename(theirs, "12", "Bass (theirs edit)")
    yield "2-conflict-keep-both", base, ours, theirs, "both"

    ours, theirs = h.clone(base), h.clone(base)
    h.set_volume(ours, "13", 0.25)
    h.add_return(theirs, "C-Theirs Return")
    yield "3-theirs-adds-return", base, ours, theirs, "fail"

    ours, theirs = h.clone(base), h.clone(base)
    h.add_return(ours, "C-Ours Return")
    h.add_return(theirs, "D-Theirs Return")
    yield "4-both-add-returns", base, ours, theirs, "fail"

    ours, theirs = h.clone(base), h.clone(base)
    h.duplicate_track(ours, "8", "Ours Drums 2")
    h.add_scene(theirs)
    yield "5-scene-and-track", base, ours, theirs, "fail"

    b = load(backup("200025"))
    theirs = h.clone(b)
    h.set_volume(theirs, "8", 0.5)
    h.set_tempo(theirs, 128.0)
    yield "6-real-history-combine", b, load(backup("200235")), theirs, "fail"


def main():
    for name, base, ours, theirs, strategy in scenarios():
        r = merge_sets(base, ours, theirs, strategy=strategy)
        out = PROJECT / f"MergeTest-{name}.als"
        r.merged.save(out)
        status = "OK" if not r.issues else f"ISSUES {r.issues}"
        print(f"== {out.name}  [{status}]")
        print(r.report())


if __name__ == "__main__":
    main()
