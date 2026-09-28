"""Command line entry point: python -m dawgit <command> ..."""

from __future__ import annotations

import argparse
import sys

from .als import LiveSet
from .diff import diff_sets


def cmd_info(args) -> int:
    s = LiveSet.load(args.file)
    print(f"{s.path.name}  [{s.creator}]  tempo {s.tempo}  scenes {s.scene_count()}  NextPointeeId {s.next_pointee_id}")
    print("\nTracks:")
    for t in s.tracks():
        group = f"  (in group {t.group_id})" if t.group_id != "-1" else ""
        print(f"  [{t.id:>3}] {t.kind:<11} {t.name}{group}")
        for d in t.device_names():
            print(f"          device  {d}")
        for c in t.clips():
            print(f"          clip    {c.kind:<9} \"{c.name}\" {c.location} {c.start:g}-{c.end:g}")
    print("\nPlugins:")
    for p in s.plugins() or ["(none)"]:
        print(f"  {p}")
    print("\nSample references:")
    for r in s.sample_refs() or []:
        pack = f" [{r.pack}]" if r.pack else ""
        print(f"  {r.relative_path or r.path}{pack}  (type {r.relative_path_type}, {r.file_size} bytes)")
    return 0


def cmd_diff(args) -> int:
    print(diff_sets(LiveSet.load(args.a), LiveSet.load(args.b)).render())
    return 0


def cmd_merge(args) -> int:
    from .merge import merge_sets

    result = merge_sets(LiveSet.load(args.base), LiveSet.load(args.ours), LiveSet.load(args.theirs),
                        strategy=args.strategy)
    print(result.report())
    if result.conflicts and args.strategy == "fail":
        print("\nmerge aborted: unresolved conflicts (use --strategy ours|theirs|both)", file=sys.stderr)
        return 1
    result.merged.save(args.output)
    print(f"\nwrote {args.output}")
    return 0


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(prog="dawgit")
    sub = p.add_subparsers(dest="cmd", required=True)

    sp = sub.add_parser("info", help="summarize a Live Set")
    sp.add_argument("file")
    sp.set_defaults(func=cmd_info)

    sp = sub.add_parser("diff", help="semantic diff of two Live Sets")
    sp.add_argument("a")
    sp.add_argument("b")
    sp.set_defaults(func=cmd_diff)

    sp = sub.add_parser("merge", help="track-level 3-way merge")
    sp.add_argument("base")
    sp.add_argument("ours")
    sp.add_argument("theirs")
    sp.add_argument("-o", "--output", required=True)
    sp.add_argument("--strategy", choices=["fail", "ours", "theirs", "both"], default="fail",
                    help="how to resolve tracks changed on both sides")
    sp.set_defaults(func=cmd_merge)

    args = p.parse_args(argv)
    return args.func(args)
