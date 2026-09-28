"""Semantic diff between two Live Sets.

The unit of comparison is the track (matched by track Id, which Live keeps
stable across saves) plus a handful of set-wide sections.
"""

from __future__ import annotations

import difflib
import math
import xml.etree.ElementTree as ET
from dataclasses import dataclass, field

from .als import LiveSet, Track, _val, device_label, is_pointee_tag
from .normalize import NOISE_ATTRS, NOISE_ELEMENTS, fingerprint

# Set-wide sections compared as a whole. Name -> paths under <LiveSet>.
GLOBAL_SECTIONS: dict[str, tuple[str, ...]] = {
    "main": ("MainTrack",),
    "transport": ("Transport",),
    "locators": ("Locators",),
    "scenes": ("Scenes",),
    "grooves": ("GroovePool",),
    "sends_pre": ("SendsPre",),
}


def section_elems(s: LiveSet, name: str) -> list[ET.Element]:
    return [s.liveset.find(p) for p in GLOBAL_SECTIONS[name]]


def section_fingerprint(s: LiveSet, name: str) -> str:
    return "|".join(fingerprint(e) for e in section_elems(s, name))


def leaves(e: ET.Element, prefix: str = "") -> dict[str, str]:
    """Flatten a subtree to {path@attr: value}, skipping noise.

    Siblings with the same tag are disambiguated by occurrence index, so the
    paths are only meaningful between structurally similar subtrees.
    """
    out: dict[str, str] = {}
    counts: dict[str, int] = {}
    for c in e:
        if c.tag in NOISE_ELEMENTS:
            continue
        n = counts.get(c.tag, 0)
        counts[c.tag] = n + 1
        path = f"{prefix}/{c.tag}" + (f"[{n}]" if n else "")
        for k, v in c.attrib.items():
            if k == "Id" or k in NOISE_ATTRS:
                continue
            out[f"{path}@{k}"] = v
        text = (c.text or "").strip()
        if text:
            out[f"{path}#text"] = text
        out.update(leaves(c, path))
    return out


@dataclass
class TrackChange:
    track_id: str
    name: str
    kind: str
    status: str  # "added" | "removed" | "modified"
    details: list[str] = field(default_factory=list)


@dataclass
class SetDiff:
    global_changes: list[str] = field(default_factory=list)
    track_changes: list[TrackChange] = field(default_factory=list)
    order_changed: bool = False

    @property
    def empty(self) -> bool:
        return not (self.global_changes or self.track_changes or self.order_changed)

    def render(self) -> str:
        if self.empty:
            return "(no changes)"
        lines = []
        for g in self.global_changes:
            lines.append(f"~ {g}")
        if self.order_changed:
            lines.append("~ track order changed")
        sym = {"added": "+", "removed": "-", "modified": "~"}
        for tc in self.track_changes:
            lines.append(f"{sym[tc.status]} {tc.kind} \"{tc.name}\" (id {tc.track_id})")
            lines.extend(f"    {d}" for d in tc.details)
        return "\n".join(lines)


def _db(gain: str) -> str:
    g = float(gain)
    return f"{20 * math.log10(g):+.1f} dB" if g > 0 else "-inf dB"


def _mixer_details(a: Track, b: Track) -> list[str]:
    out = []
    ma, mb = a.elem.find("DeviceChain/Mixer"), b.elem.find("DeviceChain/Mixer")
    va, vb = _val(ma, "Volume/Manual"), _val(mb, "Volume/Manual")
    if va != vb:
        out.append(f"mixer volume: {_db(va)} -> {_db(vb)}")
    for label, path in (("pan", "Pan/Manual"), ("track on", "Speaker/Manual"), ("solo", "SoloSink")):
        va, vb = _val(ma, path), _val(mb, path)
        if va != vb:
            out.append(f"mixer {label}: {va} -> {vb}")
    sa = [_val(h, "Send/Manual") for h in ma.iterfind("Sends/TrackSendHolder")]
    sb = [_val(h, "Send/Manual") for h in mb.iterfind("Sends/TrackSendHolder")]
    if sa != sb:
        out.append(f"sends: {sa} -> {sb}")
    for label, path in (("audio out", "DeviceChain/AudioOutputRouting/Target"),
                        ("audio in", "DeviceChain/AudioInputRouting/Target"),
                        ("midi in", "DeviceChain/MidiInputRouting/Target")):
        va, vb = _val(a.elem, path), _val(b.elem, path)
        if va != vb:
            out.append(f"routing {label}: {va} -> {vb}")
    return out


def _device_details(a: Track, b: Track) -> list[str]:
    da, db = a.devices(), b.devices()
    na, nb = [device_label(d) for d in da], [device_label(d) for d in db]
    out = []
    if na != nb:
        for op, i1, i2, j1, j2 in difflib.SequenceMatcher(a=na, b=nb).get_opcodes():
            if op in ("delete", "replace"):
                out.extend(f"- device {n}" for n in na[i1:i2])
            if op in ("insert", "replace"):
                out.extend(f"+ device {n}" for n in nb[j1:j2])
        return out
    for dev_a, dev_b, label in zip(da, db, na):
        if fingerprint(dev_a) == fingerprint(dev_b):
            continue
        if dev_a.tag == "PluginDevice":
            out.extend(_plugin_details(dev_a, dev_b, label))
            continue
        la, lb = leaves(dev_a), leaves(dev_b)
        params = sorted({k.split("/")[1] for k in set(la) | set(lb)
                         if la.get(k) != lb.get(k) and k.endswith("/Manual@Value")})
        changed = [k for k in set(la) | set(lb) if la.get(k) != lb.get(k)]
        if params:
            shown = ", ".join(params[:6]) + (" ..." if len(params) > 6 else "")
            out.append(f"~ device {label}: params {shown}")
        elif changed:
            out.append(f"~ device {label}: {len(changed)} internal value(s) changed")
    return out


def _plugin_details(a: ET.Element, b: ET.Element, label: str) -> list[str]:
    out = []
    pa = {_val(p, "ParameterName"): _val(p, "ParameterValue/Manual") for p in a.iter("PluginFloatParameter")}
    pb = {_val(p, "ParameterName"): _val(p, "ParameterValue/Manual") for p in b.iter("PluginFloatParameter")}
    params = [k for k in pa.keys() | pb.keys() if pa.get(k) != pb.get(k)]
    if params:
        out.append(f"~ device {label}: exposed params {', '.join(sorted(params)[:6])}")
    state_a = [(e.text or "").strip() for e in a.iter("Buffer")]
    state_b = [(e.text or "").strip() for e in b.iter("Buffer")]
    if state_a != state_b:
        out.append(f"~ device {label}: plugin state changed (opaque)")
    if not out:
        out.append(f"~ device {label}: settings changed")
    return out


CLIP_PARTS = {"Notes": "notes", "Fades": "fades", "Loop": "loop", "WarpMarkers": "warp markers",
              "Envelopes": "clip envelopes", "Name": "name", "Color": "color"}


def _clip_details(a: Track, b: Track) -> list[str]:
    def key(c):
        return (c.kind, c.name, c.location, c.start, c.end)
    ca = {key(c): c for c in a.clips()}
    cb = {key(c): c for c in b.clips()}
    out = []
    for k in sorted(ca.keys() - cb.keys(), key=str):
        c = ca[k]
        out.append(f"- clip \"{c.name}\" {c.location} {c.start:g}-{c.end:g}")
    for k in sorted(cb.keys() - ca.keys(), key=str):
        c = cb[k]
        out.append(f"+ clip \"{c.name}\" {c.location} {c.start:g}-{c.end:g}")
    for k in sorted(ca.keys() & cb.keys(), key=str):
        x, y = ca[k].elem, cb[k].elem
        if fingerprint(x) == fingerprint(y):
            continue
        parts = []
        for child in y:
            other = x.find(child.tag)
            if fingerprint(other) != fingerprint(child) and child.tag not in NOISE_ELEMENTS:
                parts.append(CLIP_PARTS.get(child.tag, child.tag))
        out.append(f"~ clip \"{cb[k].name}\" {cb[k].location}: {', '.join(parts) or 'content'} changed")
    return out


def _param_labels(unit: ET.Element) -> dict[str, str]:
    """Pointee id -> human label such as 'Reverb: DryWet' or 'Mixer: Volume'."""
    labels: dict[str, str] = {}

    def walk(e: ET.Element, owner: str) -> None:
        for c in e:
            o = owner
            if e.tag == "Devices":
                o = device_label(c)
            elif c.tag == "Mixer":
                o = "Mixer"
            elif c.tag == "TrackSendHolder":
                o = f"Send {chr(ord('A') + int(c.get('Id', '0')))}"
            if "Id" in c.attrib and is_pointee_tag(c.tag):
                labels[c.get("Id")] = f"{o}: {e.tag}" if o else e.tag
            walk(c, o)

    walk(unit, "")
    return labels


def _envelopes(unit: ET.Element) -> dict[str, str]:
    labels = _param_labels(unit)
    out = {}
    envs = unit.find("AutomationEnvelopes/Envelopes")
    for env in envs if envs is not None else []:
        pid = _val(env, "EnvelopeTarget/PointeeId")
        out[labels.get(pid, f"target {pid}")] = fingerprint(env.find("Automation"))
    return out


def _automation_details(a: ET.Element, b: ET.Element) -> list[str]:
    ea, eb = _envelopes(a), _envelopes(b)
    out = [f"- automation {k}" for k in sorted(ea.keys() - eb.keys())]
    out += [f"+ automation {k}" for k in sorted(eb.keys() - ea.keys())]
    out += [f"~ automation {k}" for k in sorted(ea.keys() & eb.keys()) if ea[k] != eb[k]]
    return out


def diff_tracks(a: Track, b: Track) -> list[str]:
    details = []
    # EffectiveName also changes when Live renumbers auto-named tracks; only
    # a UserName change is a rename.
    if _val(a.elem, "Name/UserName") != _val(b.elem, "Name/UserName"):
        details.append(f"renamed: \"{a.name}\" -> \"{b.name}\"")
    if a.group_id != b.group_id:
        details.append(f"group: {a.group_id} -> {b.group_id}")
    details += _device_details(a, b)
    details += _clip_details(a, b)
    details += _mixer_details(a, b)
    details += _automation_details(a.elem, b.elem)
    if not details:
        details.append("~ other changes")
    return details


def diff_sets(a: LiveSet, b: LiveSet) -> SetDiff:
    d = SetDiff()
    if a.tempo != b.tempo:
        d.global_changes.append(f"tempo: {a.tempo} -> {b.tempo}")
    for name in GLOBAL_SECTIONS:
        if name == "main" and a.tempo != b.tempo:
            continue
        if section_fingerprint(a, name) != section_fingerprint(b, name):
            d.global_changes.append(f"{name} changed")
    main_a, main_b = a.liveset.find("MainTrack"), b.liveset.find("MainTrack")
    d.global_changes += [f"main track {x}" for x in _automation_details(main_a, main_b)]

    ta, tb = a.track_by_id(), b.track_by_id()
    for tid, t in ta.items():
        if tid not in tb:
            d.track_changes.append(TrackChange(tid, t.name, t.kind, "removed"))
    for tid, t in tb.items():
        if tid not in ta:
            d.track_changes.append(TrackChange(tid, t.name, t.kind, "added",
                                               [f"devices: {', '.join(t.device_names()) or '(none)'}"]))
        elif fingerprint(ta[tid].elem) != fingerprint(t.elem):
            d.track_changes.append(TrackChange(tid, t.name, t.kind, "modified", diff_tracks(ta[tid], t)))

    common = [tid for tid in ta if tid in tb]
    d.order_changed = common != [tid for tid in tb if tid in ta]
    return d
