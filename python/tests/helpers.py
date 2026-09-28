"""Fixture access and helpers that mimic edits Live makes to a set."""

from __future__ import annotations

import copy
from pathlib import Path

from dawgit.als import LiveSet, Track, is_pointee_tag

ROOT = Path(__file__).resolve().parent.parent.parent
PROJECT = ROOT / "SampleProjects" / "SampleAbletonProject Project"
CURRENT = PROJECT / "SampleAbletonProject.als"


def backup(stamp: str) -> Path:
    return PROJECT / "Backup" / f"SampleAbletonProject [2026-09-28 {stamp}].als"


def load(path: Path) -> LiveSet:
    return LiveSet.load(path)


def clone(s: LiveSet) -> LiveSet:
    return LiveSet(copy.deepcopy(s.root))


def track(s: LiveSet, tid: str) -> Track:
    return s.track_by_id()[tid]


def set_volume(s: LiveSet, tid: str, gain: float) -> None:
    track(s, tid).elem.find("DeviceChain/Mixer/Volume/Manual").set("Value", repr(gain))


def set_tempo(s: LiveSet, bpm: float) -> None:
    s.liveset.find("MainTrack/DeviceChain/Mixer/Tempo/Manual").set("Value", repr(bpm))


def rename(s: LiveSet, tid: str, name: str) -> None:
    track(s, tid).elem.find("Name/EffectiveName").set("Value", name)
    track(s, tid).elem.find("Name/UserName").set("Value", name)


def _allocate_pointees(s: LiveSet, elem) -> None:
    """Give every pointee in elem a fresh id, like Live does for new objects."""
    next_id = s.next_pointee_id
    mapping = {}
    for e in elem.iter():
        if "Id" in e.attrib and is_pointee_tag(e.tag):
            mapping[e.get("Id")] = str(next_id)
            e.set("Id", str(next_id))
            next_id += 1
    for ref in elem.iter("PointeeId"):
        if ref.get("Value") in mapping:
            ref.set("Value", mapping[ref.get("Value")])
    s.next_pointee_id = next_id


def duplicate_track(s: LiveSet, tid: str, name: str) -> str:
    """Mimic Live's 'Duplicate track': new track id, fresh pointee ids."""
    src = track(s, tid).elem
    new = copy.deepcopy(src)
    new_id = str(1 + max(int(t.id) for t in s.tracks()))
    new.set("Id", new_id)
    _allocate_pointees(s, new)
    kids = list(s.tracks_elem)
    s.tracks_elem.insert(kids.index(src) + 1, new)
    rename(s, new_id, name)
    return new_id


def add_return(s: LiveSet, name: str) -> str:
    """Mimic adding a return track: new return + a send on every track."""
    returns = [t for t in s.tracks() if t.kind == "ReturnTrack"]
    new_id = duplicate_track(s, returns[-1].id, name)
    for t in s.tracks():
        sends = t.elem.find("DeviceChain/Mixer/Sends")
        holder = copy.deepcopy(sends[-1])
        holder.set("Id", str(len(sends)))
        _allocate_pointees(s, holder)
        sends.append(holder)
    pre = s.liveset.find("SendsPre")
    e = copy.deepcopy(pre[-1])
    e.set("Id", str(len(pre)))
    pre.append(e)
    return new_id


def add_scene(s: LiveSet) -> None:
    scenes = s.liveset.find("Scenes")
    scene = copy.deepcopy(scenes[-1])
    scene.set("Id", str(len(scenes)))
    scenes.append(scene)
    for t in s.tracks():
        if t.kind == "ReturnTrack":
            continue
        lists = ["DeviceChain/FreezeSequencer/ClipSlotList"]
        lists.append("Slots" if t.kind == "GroupTrack" else "DeviceChain/MainSequencer/ClipSlotList")
        for path in lists:
            slots = t.elem.find(path)
            slot = copy.deepcopy(slots[-1])
            slot.set("Id", str(len(slots)))
            slots.append(slot)


def delete_track(s: LiveSet, tid: str) -> None:
    s.tracks_elem.remove(track(s, tid).elem)


CURRENT_V2 = PROJECT / "SampleAbletonProject_v2.als"


def backup_v2(stamp: str) -> Path:
    return PROJECT / "Backup" / f"SampleAbletonProject_v2 [2026-09-28 {stamp}].als"
