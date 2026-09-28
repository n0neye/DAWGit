"""Track-level 3-way merge of Live Sets.

Units of merge:
  * each track (matched by track Id; Live keeps it stable across saves)
  * each (track, return track) send, merged independently from the track body
    so adding a return track on one side does not conflict with every track
  * set-wide sections: main track (tempo, master chain), transport, locators,
    scenes, groove pool

After units are chosen the result is repaired so Live can load it: send and
clip-slot lists are rebuilt to match return tracks / scenes, pointee ids that
collide are renumbered (with their envelope references), dangling automation
is dropped and NextPointeeId is advanced.
"""

from __future__ import annotations

import copy
import re
import xml.etree.ElementTree as ET
from dataclasses import dataclass, field

from .als import LiveSet, Track, is_pointee_tag
from .diff import GLOBAL_SECTIONS, section_elems, section_fingerprint
from .normalize import fingerprint
from .validate import ROUTING_TRACK_RE, units, validate

STRATEGIES = ("fail", "ours", "theirs", "both")
# Sends are merged per (track, return); group membership and output routing
# ("placement") separately; clip slots are compared by content only.
PLACEMENT_PATHS = ("TrackGroupId", "DeviceChain/AudioOutputRouting")
TRACK_SKIP = frozenset({"Sends", "ClipSlotList", "Slots", "TrackGroupId", "AudioOutputRouting"})


@dataclass
class Conflict:
    unit: str
    description: str
    resolution: str


@dataclass
class MergeResult:
    merged: LiveSet
    log: list[str] = field(default_factory=list)
    conflicts: list[Conflict] = field(default_factory=list)
    issues: list[str] = field(default_factory=list)

    def report(self) -> str:
        lines = [f"  {line}" for line in self.log] or ["  (nothing to merge)"]
        if self.conflicts:
            lines.append(f"\n{len(self.conflicts)} conflict(s):")
            lines += [f"  ! {c.unit}: {c.description} -> {c.resolution}" for c in self.conflicts]
        if self.issues:
            lines.append("\nvalidation issues:")
            lines += [f"  x {i}" for i in self.issues]
        return "\n".join(lines)


def _three_way(fb, fo, ft) -> str:
    if fo == ft or ft == fb:
        return "ours"
    if fo == fb:
        return "theirs"
    return "conflict"


def _track_fp(t: Track | None) -> str | None:
    """Track identity for 3-way decisions.

    Empty clip slots are left out so adding/removing a scene on one side does
    not count as modifying every track; the slot lists are refitted afterwards.
    """
    if t is None:
        return None
    parts = [fingerprint(t.elem, TRACK_SKIP)]
    for seq in ("MainSequencer", "FreezeSequencer"):
        slots = t.elem.find(f"DeviceChain/{seq}/ClipSlotList")
        for i, slot in enumerate(slots if slots is not None else []):
            value = slot.find("ClipSlot/Value")
            if value is not None and len(value):
                parts.append(f"{seq}[{i}]={fingerprint(slot)}")
    return "|".join(parts)


def _placement_fp(elem: ET.Element) -> str:
    return "|".join(fingerprint(elem.find(p)) for p in PLACEMENT_PATHS)


def _retail(parent: ET.Element) -> None:
    """Re-indent children after structural edits (Live writes tab indentation)."""
    kids = list(parent)
    if not kids:
        parent.text = None
        return
    indent = parent.text if parent.text and parent.text.strip() == "" else "\n"
    for k in kids:
        k.tail = indent
    kids[-1].tail = indent[:-1] if indent.endswith("\t") else indent


class _Merger:
    def __init__(self, base: LiveSet, ours: LiveSet, theirs: LiveSet, strategy: str):
        if strategy not in STRATEGIES:
            raise ValueError(f"unknown strategy {strategy!r}")
        self.base, self.ours, self.theirs = base, ours, theirs
        self.sides = {"base": base, "ours": ours, "theirs": theirs}
        self.strategy = strategy
        self.merged = LiveSet(copy.deepcopy(ours.root))
        self.result = MergeResult(self.merged)
        # Nodes copied from theirs or synthesized; their pointee ids yield to
        # ours' on collision. Recorded at copy time because ours' nodes may later
        # be attached under an imported track (e.g. send holders).
        self.imported: set[int] = set()
        self._keepalive: list[ET.Element] = []  # keep id() values unique
        # merged track element -> {side name: track id in that side}
        self.ident: dict[ET.Element, dict[str, str]] = {}
        # theirs track id -> new id, for theirs tracks renumbered on import
        self.theirs_renumber: dict[str, str] = {}
        self.next_track_id = 1 + max(
            (int(t.id) for s in (base, ours, theirs) for t in s.tracks()), default=0)

    # --- helpers ---
    def log(self, msg: str) -> None:
        self.result.log.append(msg)

    def conflict(self, unit: str, description: str, resolution: str) -> None:
        self.result.conflicts.append(Conflict(unit, description, resolution))

    def _import(self, elem: ET.Element) -> ET.Element:
        c = copy.deepcopy(elem)
        nodes = list(c.iter())
        self._keepalive.extend(nodes)  # detached nodes must not free their id()
        self.imported.update(id(e) for e in nodes)
        return c

    def _ident_for(self, tid: str) -> dict[str, str]:
        """Which side's track with this id is the same logical track as ours'.

        Ids are only a shared identity when the track existed in base; two
        tracks created independently on each side can share an id by chance.
        """
        tb, to, tt = (s.track_by_id() for s in (self.base, self.ours, self.theirs))
        ident = {"ours": tid} if tid in to else {}
        if tid in tb:
            ident["base"] = tid
        if tid in tt and (tid in tb or tid not in to or _track_fp(to[tid]) == _track_fp(tt[tid])):
            ident["theirs"] = tid
        return ident

    def _new_track_id(self) -> str:
        tid = str(self.next_track_id)
        self.next_track_id += 1
        return tid

    # --- steps ---
    def merge_globals(self) -> None:
        for name in GLOBAL_SECTIONS:
            if name == "sends_pre":
                continue  # rebuilt from the merged return tracks
            fb, fo, ft = (section_fingerprint(s, name) for s in (self.base, self.ours, self.theirs))
            decision = _three_way(fb, fo, ft)
            if decision == "conflict":
                take_theirs = self.strategy == "theirs"
                self.conflict(name, "changed on both sides", "theirs" if take_theirs else "ours")
                decision = "theirs" if take_theirs else "ours"
            elif decision == "theirs":
                self.log(f"{name}: took theirs")
            if decision == "theirs":
                ls = self.merged.liveset
                for mine, their in zip(section_elems(self.merged, name), section_elems(self.theirs, name)):
                    idx = list(ls).index(mine)
                    ls.remove(mine)
                    ls.insert(idx, self._import(their))

    def merge_tracks(self) -> None:
        tb, to, tt = self.base.track_by_id(), self.ours.track_by_id(), self.theirs.track_by_id()
        tracks_elem = self.merged.tracks_elem
        merged_by_id = {t.id: t.elem for t in self.merged.tracks()}
        for elem in merged_by_id.values():
            self.ident[elem] = self._ident_for(elem.get("Id"))

        # theirs track id -> merged element representing it (for placement)
        placed: dict[str, ET.Element] = {tid: e for tid, e in merged_by_id.items() if "theirs" in self.ident[e]}
        theirs_order = [t.id for t in self.theirs.tracks()]

        def insert_after_theirs_neighbour(tid: str, elem: ET.Element) -> None:
            kids = list(tracks_elem)
            idx = 0
            for prev in reversed(theirs_order[:theirs_order.index(tid)]):
                if prev in placed and placed[prev] in kids:
                    idx = kids.index(placed[prev]) + 1
                    break
            # Tracks ours added at the same spot come first.
            while idx < len(kids) and set(self.ident.get(kids[idx], {})) == {"ours"}:
                idx += 1
            tracks_elem.insert(idx, elem)
            placed[tid] = elem

        def add_theirs(tid: str, *, rename: str | None = None, after: ET.Element | None = None) -> ET.Element:
            elem = self._import(tt[tid].elem)
            new_id = tid
            if tid in merged_by_id or rename is not None:
                new_id = self._new_track_id()
                elem.set("Id", new_id)
                if rename is None:
                    self.theirs_renumber[tid] = new_id
            if rename is not None:
                # EffectiveName is derived by Live; an empty UserName means auto-named.
                user, eff = elem.find("Name/UserName"), elem.find("Name/EffectiveName")
                name = user.get("Value") or re.sub(r"^\d+-", "", eff.get("Value"))
                user.set("Value", name + rename)
                eff.set("Value", name + rename)
            self.ident[elem] = {"theirs": tid}
            merged_by_id[new_id] = elem
            if after is not None:
                tracks_elem.insert(list(tracks_elem).index(after) + 1, elem)
            else:
                insert_after_theirs_neighbour(tid, elem)
            return elem

        def replace_with_theirs(tid: str) -> None:
            old = merged_by_id[tid]
            new = self._import(tt[tid].elem)
            idx = list(tracks_elem).index(old)
            tracks_elem.remove(old)
            tracks_elem.insert(idx, new)
            self.ident[new] = self.ident.pop(old)
            merged_by_id[tid] = new
            placed[tid] = new

        def remove(tid: str) -> None:
            elem = merged_by_id.pop(tid)
            tracks_elem.remove(elem)
            self.ident.pop(elem, None)

        all_ids = list(to) + [tid for tid in tt if tid not in to] + [tid for tid in tb if tid not in to and tid not in tt]
        for tid in all_ids:
            b, o, t = tb.get(tid), to.get(tid), tt.get(tid)
            decision = _three_way(_track_fp(b), _track_fp(o), _track_fp(t))
            label = f"{(t or o or b).kind} \"{(t or o or b).name}\""
            if decision == "ours":
                if o is not None and b is not None and _track_fp(o) != _track_fp(b):
                    self.log(f"{label}: kept ours")
                continue
            if decision == "theirs":
                if t is None:
                    remove(tid)
                    self.log(f"{label}: removed (deleted in theirs)")
                elif o is None:
                    add_theirs(tid)
                    self.log(f"{label}: added from theirs")
                else:
                    replace_with_theirs(tid)
                    self.log(f"{label}: took theirs")
                continue

            # conflict
            if b is None:
                # Both sides created a track that happens to share an id.
                add_theirs(tid)
                self.log(f"{label}: added from theirs (id {tid} also used by ours, renumbered)")
                continue
            if o is not None and t is not None:
                what = "modified on both sides"
            elif o is None:
                what = "deleted in ours, modified in theirs"
            else:
                what = "modified in ours, deleted in theirs"
            s = self.strategy
            if s == "theirs":
                if t is None:
                    remove(tid)
                elif o is None:
                    add_theirs(tid)
                else:
                    replace_with_theirs(tid)
                self.conflict(label, what, "theirs")
            elif s == "both":
                if o is not None and t is not None:
                    add_theirs(tid, rename=" [theirs]", after=merged_by_id[tid])
                    self.conflict(label, what, "kept both (theirs added as a copy)")
                elif o is None:
                    add_theirs(tid)
                    self.conflict(label, what, "restored theirs")
                else:
                    self.conflict(label, what, "kept ours")
            else:
                self.conflict(label, what, "kept ours" if s == "ours" else "unresolved (kept ours)")

    def merge_placement(self) -> None:
        """Merge group membership and output routing separately from the track body."""
        sides = {"base": self.base.track_by_id(), "ours": self.ours.track_by_id(),
                 "theirs": self.theirs.track_by_id()}
        for elem, ident in list(self.ident.items()):
            if not {"base", "ours", "theirs"} <= ident.keys():
                continue
            src = {name: sides[name][ident[name]].elem for name in ("base", "ours", "theirs")}
            decision = _three_way(*(_placement_fp(src[n]) for n in ("base", "ours", "theirs")))
            label = f"{elem.tag} \"{Track(elem).name}\""
            if decision == "conflict":
                decision = "theirs" if self.strategy == "theirs" else "ours"
                self.conflict(f"{label} placement", "group/output changed on both sides", decision)
            elif decision == "theirs" and _placement_fp(src["theirs"]) != _placement_fp(src["ours"]):
                self.log(f"{label}: took theirs' group/output")
            donor = src[decision]
            if _placement_fp(donor) != _placement_fp(elem):
                for path in PLACEMENT_PATHS:
                    mine, new = elem.find(path), copy.deepcopy(donor.find(path))
                    parent = elem.find(path.rsplit("/", 1)[0]) if "/" in path else elem
                    idx = list(parent).index(mine)
                    parent.remove(mine)
                    parent.insert(idx, new)
                if decision == "theirs":
                    g = elem.find("TrackGroupId")
                    g.set("Value", self.theirs_renumber.get(g.get("Value"), g.get("Value")))

    def merge_order(self) -> None:
        """3-way merge of track order, then keep groups contiguous and returns last."""
        tracks_elem = self.merged.tracks_elem
        current = list(tracks_elem)
        orders = {name: [t.id for t in s.tracks()] for name, s in self.sides.items()}
        common = [tid for tid in orders["base"] if tid in orders["ours"] and tid in orders["theirs"]]
        ob, oo, ot = ([tid for tid in orders[n] if tid in common] for n in ("base", "ours", "theirs"))
        decision = _three_way(ob, oo, ot)
        if decision == "conflict":
            decision = "theirs" if self.strategy == "theirs" else "ours"
            self.conflict("track order", "reordered on both sides", decision)
        if decision == "theirs" and ot != oo:
            self.log("track order: took theirs")
            rank = {tid: i for i, tid in enumerate(orders["theirs"])}
            ordered = sorted((e for e in current if "theirs" in self.ident.get(e, {})),
                             key=lambda e: rank[self.ident[e]["theirs"]])
            # Ours-only tracks stay right after their previous neighbour in ours' layout.
            for i, e in enumerate(current):
                if e in ordered:
                    continue
                prev = next((p for p in reversed(current[:i]) if p in ordered), None)
                ordered.insert(ordered.index(prev) + 1 if prev is not None else 0, e)
            current = ordered

        # Group members follow their group track contiguously; returns go last.
        groups = {e.get("Id") for e in current if e.tag == "GroupTrack"}
        children: dict[str, list[ET.Element]] = {}
        roots = []
        for e in current:
            if e.tag == "ReturnTrack":
                continue
            gid = Track(e).group_id
            (children.setdefault(gid, []) if gid in groups else roots).append(e)
        final: list[ET.Element] = []

        def emit(e: ET.Element) -> None:
            final.append(e)
            for c in children.get(e.get("Id"), []) if e.tag == "GroupTrack" else []:
                emit(c)

        for e in roots:
            emit(e)
        final += [e for e in current if e.tag == "ReturnTrack"]
        for k in list(tracks_elem):
            tracks_elem.remove(k)
        for k in final:
            tracks_elem.append(k)
        _retail(tracks_elem)

    def fix_references(self) -> None:
        """Point imported theirs tracks at renumbered track ids; drop missing groups."""
        tracks = self.merged.tracks()
        ids = {t.id for t in tracks}
        groups = {t.id for t in tracks if t.kind == "GroupTrack"}
        for t in tracks:
            if id(t.elem) in self.imported and "theirs" in self.ident.get(t.elem, {}) and self.theirs_renumber:
                g = t.elem.find("TrackGroupId")
                if g is not None and g.get("Value") in self.theirs_renumber:
                    g.set("Value", self.theirs_renumber[g.get("Value")])
                for target in t.elem.iter("Target"):
                    v = target.get("Value", "")
                    m = ROUTING_TRACK_RE.search(v)
                    if m and m.group(1) in self.theirs_renumber:
                        target.set("Value", v[:m.start(1)] + self.theirs_renumber[m.group(1)] + v[m.end(1):])
            g = t.elem.find("TrackGroupId")
            if g is not None and g.get("Value") != "-1" and g.get("Value") not in groups:
                self.log(f"track \"{t.name}\": group {g.get('Value')} no longer exists, ungrouped")
                g.set("Value", "-1")
            for target in t.elem.iter("Target"):
                m = ROUTING_TRACK_RE.search(target.get("Value", ""))
                if m and m.group(1) not in ids:
                    self.log(f"track \"{t.name}\": routing {target.get('Value')} points to a removed track")

    def rebuild_sends(self) -> None:
        returns = [t for t in self.merged.tracks() if t.kind == "ReturnTrack"]
        side_returns = {name: [t.id for t in s.tracks() if t.kind == "ReturnTrack"] for name, s in self.sides.items()}
        side_tracks = {name: s.track_by_id() for name, s in self.sides.items()}

        def holder(side: str, track_ident: dict, return_ident: dict) -> ET.Element | None:
            tid, rid = track_ident.get(side), return_ident.get(side)
            if tid is None or rid is None or rid not in side_returns[side]:
                return None
            holders = side_tracks[side][tid].elem.findall("DeviceChain/Mixer/Sends/TrackSendHolder")
            idx = side_returns[side].index(rid)
            return holders[idx] if idx < len(holders) else None

        template = next((h for s in (self.merged, *self.sides.values())
                         for h in s.root.iter("TrackSendHolder")), None)

        for t in self.merged.tracks():
            sends = t.elem.find("DeviceChain/Mixer/Sends")
            if sends is None:
                continue
            tident = self.ident.get(t.elem, self._ident_for(t.id))
            new_holders = []
            for r in returns:
                rident = self.ident.get(r.elem, self._ident_for(r.id))
                hb, ho, ht = (holder(side, tident, rident) for side in ("base", "ours", "theirs"))
                decision = _three_way(*(fingerprint(h) if h is not None else None for h in (hb, ho, ht)))
                if decision == "conflict":
                    decision = "theirs" if self.strategy == "theirs" else "ours"
                    self.conflict(f"send \"{t.name}\" -> \"{r.name}\"", "changed on both sides", decision)
                chosen = ht if decision == "theirs" else ho
                if chosen is None and decision == "ours" and ho is None:
                    chosen = ht  # track or return unknown to ours: theirs is the only source
                if chosen is not None:
                    h = self._import(chosen) if chosen is ht else copy.deepcopy(chosen)
                else:
                    h = self._import(template)
                    send = h.find("Send")
                    send.find("Manual").set("Value", send.find("MidiControllerRange/Min").get("Value"))
                new_holders.append(h)
            for old in list(sends):
                sends.remove(old)
            for i, h in enumerate(new_holders):
                h.set("Id", str(i))
                sends.append(h)
            _retail(sends)

        sends_pre = self.merged.liveset.find("SendsPre")
        if sends_pre is not None:
            values = []
            for r in returns:
                rident = self.ident.get(r.elem, self._ident_for(r.id))
                side = "ours" if "ours" in rident else "theirs"
                idx = side_returns[side].index(rident[side])
                src = self.sides[side].liveset.find("SendsPre")
                values.append(src[idx].get("Value") if idx < len(src) else "false")
            template_pre = sends_pre[0] if len(sends_pre) else ET.Element("SendPreBool")
            for old in list(sends_pre):
                sends_pre.remove(old)
            for i, v in enumerate(values):
                e = copy.deepcopy(template_pre)
                e.set("Id", str(i))
                e.set("Value", v)
                sends_pre.append(e)
            _retail(sends_pre)

    def fit_clip_slots(self) -> None:
        scenes = self.merged.scene_count()
        empty = None
        for s in (self.merged, self.ours, self.theirs, self.base):
            empty = next((cs for cs in s.root.iter("ClipSlot")
                          if cs.find("ClipSlot/Value") is not None and len(cs.find("ClipSlot/Value")) == 0), None)
            if empty is not None:
                break
        group_slot = next((gs for s in (self.merged, self.ours, self.theirs, self.base)
                           for gs in s.root.iter("GroupTrackSlot")), None)
        for t in self.merged.tracks():
            if t.kind == "GroupTrack":
                slots = t.elem.find("Slots")
                if group_slot is not None and slots is not None:
                    while len(slots) < scenes:
                        slots.append(copy.deepcopy(group_slot))
                    del slots[scenes:]
                    for i, gs in enumerate(slots):
                        gs.set("Id", str(i))
                    _retail(slots)
            if t.kind == "ReturnTrack":
                continue
            for seq in ("MainSequencer", "FreezeSequencer"):
                slots = t.elem.find(f"DeviceChain/{seq}/ClipSlotList")
                if slots is None or len(slots) == scenes:
                    continue
                while len(slots) < scenes and empty is not None:
                    slots.append(copy.deepcopy(empty))
                while len(slots) > scenes and len(slots[-1].find("ClipSlot/Value")) == 0:
                    slots.remove(slots[-1])
                if len(slots) != scenes:
                    self.log(f"track \"{t.name}\": has clips in scenes that no longer exist")
                for i, cs in enumerate(slots):
                    cs.set("Id", str(i))
                _retail(slots)

    def renumber_pointees(self) -> None:
        imported_nodes = self.imported
        all_pointees = list(self.merged.pointee_elements())
        used = {e.get("Id") for e in all_pointees if id(e) not in imported_nodes}
        next_id = 1 + max([int(e.get("Id")) for e in all_pointees] +
                          [self.ours.next_pointee_id - 1, self.theirs.next_pointee_id - 1])

        unit_of = {}
        for unit in units(self.merged):
            if unit is not None:
                for e in unit.iter():
                    unit_of[id(e)] = unit

        remaps: dict[ET.Element, dict[str, str]] = {}
        renumbered = 0
        for e in all_pointees:
            if id(e) not in imported_nodes:
                continue
            old = e.get("Id")
            if old in used:
                new = str(next_id)
                next_id += 1
                e.set("Id", new)
                renumbered += 1
                unit = unit_of.get(id(e))
                if unit is not None:
                    remaps.setdefault(unit, {})[old] = new
                old = new
            used.add(old)

        for unit, mapping in remaps.items():
            kept = {e.get("Id") for e in unit.iter()
                    if "Id" in e.attrib and is_pointee_tag(e.tag) and id(e) not in imported_nodes}
            for ref in unit.iter("PointeeId"):
                v = ref.get("Value")
                if v in mapping and v not in kept:
                    ref.set("Value", mapping[v])
        if renumbered:
            self.log(f"renumbered {renumbered} colliding automation/modulation target id(s)")
        self.merged.next_pointee_id = next_id

    def drop_dangling_envelopes(self) -> None:
        for unit in units(self.merged):
            if unit is None:
                continue
            local = {e.get("Id") for e in unit.iter() if "Id" in e.attrib and is_pointee_tag(e.tag)}
            for parent in list(unit.iter()):
                for env in list(parent):
                    ref = env.find("EnvelopeTarget/PointeeId")
                    if ref is not None and ref.get("Value") not in local:
                        parent.remove(env)
                        _retail(parent)
                        self.log(f"{unit.tag} {unit.get('Id', '')}: dropped automation for a removed parameter")

    def run(self) -> MergeResult:
        self.merge_globals()
        self.merge_tracks()
        self.merge_placement()
        self.fix_references()
        self.merge_order()
        self.rebuild_sends()
        self.fit_clip_slots()
        self.renumber_pointees()
        self.drop_dangling_envelopes()
        self.result.issues = validate(self.merged)
        return self.result


def merge_sets(base: LiveSet, ours: LiveSet, theirs: LiveSet, strategy: str = "fail") -> MergeResult:
    """3-way merge; ours is the starting point, theirs' changes are applied on top."""
    return _Merger(base, ours, theirs, strategy).run()
