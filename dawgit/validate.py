"""Structural invariants a Live Set must satisfy for Live to load it cleanly.

These were derived from real sets; a merge result must pass all of them.
"""

from __future__ import annotations

import re
import xml.etree.ElementTree as ET
from collections import Counter

from .als import LiveSet, is_pointee_tag

ROUTING_TRACK_RE = re.compile(r"Track\.(\d+)")


def units(s: LiveSet) -> list[ET.Element]:
    """Top-level containers that own their pointee ids and envelope references."""
    return [t.elem for t in s.tracks()] + [s.liveset.find("MainTrack"), s.liveset.find("PreHearTrack")]


def validate(s: LiveSet) -> list[str]:
    issues: list[str] = []

    ids = [int(e.get("Id")) for e in s.pointee_elements()]
    dups = [i for i, n in Counter(ids).items() if n > 1]
    if dups:
        issues.append(f"duplicate pointee ids: {sorted(dups)[:10]}{' ...' if len(dups) > 10 else ''}")
    if ids and s.next_pointee_id <= max(ids):
        issues.append(f"NextPointeeId {s.next_pointee_id} <= max pointee id {max(ids)}")

    for unit in units(s):
        if unit is None:
            continue
        local = {e.get("Id") for e in unit.iter() if "Id" in e.attrib and is_pointee_tag(e.tag)}
        for ref in unit.iter("PointeeId"):
            if ref.get("Value") not in local:
                issues.append(f"{unit.tag} {unit.get('Id', '')}: dangling PointeeId {ref.get('Value')}")

    tracks = s.tracks()
    track_ids = [t.id for t in tracks]
    dup_tracks = [i for i, n in Counter(track_ids).items() if n > 1]
    if dup_tracks:
        issues.append(f"duplicate track ids: {dup_tracks}")

    kinds = [t.kind for t in tracks]
    if "ReturnTrack" in kinds and any(k != "ReturnTrack" for k in kinds[kinds.index("ReturnTrack"):]):
        issues.append("return tracks must come after all other tracks")

    scenes = s.scene_count()
    n_returns = kinds.count("ReturnTrack")
    group_ids = {t.id for t in tracks if t.kind == "GroupTrack"}
    for t in tracks:
        # Return tracks have no clip slots.
        for seq in () if t.kind == "ReturnTrack" else ("MainSequencer", "FreezeSequencer"):
            slots = t.elem.find(f"DeviceChain/{seq}/ClipSlotList")
            if slots is not None and len(slots) != scenes:
                issues.append(f"track {t.id} {seq}: {len(slots)} clip slots, {scenes} scenes")
        holders = t.elem.findall("DeviceChain/Mixer/Sends/TrackSendHolder")
        if len(holders) != n_returns:
            issues.append(f"track {t.id}: {len(holders)} sends, {n_returns} return tracks")
        if t.group_id != "-1" and t.group_id not in group_ids:
            issues.append(f"track {t.id}: TrackGroupId {t.group_id} does not exist")
        for target in t.elem.iter("Target"):
            m = ROUTING_TRACK_RE.search(target.get("Value", ""))
            if m and m.group(1) not in track_ids:
                issues.append(f"track {t.id}: routing to missing track {target.get('Value')}")

    sends_pre = s.liveset.find("SendsPre")
    if sends_pre is not None and len(sends_pre) != n_returns:
        issues.append(f"SendsPre has {len(sends_pre)} entries, {n_returns} return tracks")

    return issues
