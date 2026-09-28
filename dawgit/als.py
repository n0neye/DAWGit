"""Load, inspect and save Ableton Live Sets (.als).

An .als file is gzip-compressed XML. Loading and saving through this module is
byte-exact: an untouched set saved back produces the identical XML payload
Live wrote (CRLF line endings, Live's attribute quoting rules).
"""

from __future__ import annotations

import gzip
import xml.etree.ElementTree as ET
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterator

XML_DECL = '<?xml version="1.0" encoding="UTF-8"?>\n'

TRACK_TAGS = ("MidiTrack", "AudioTrack", "GroupTrack", "ReturnTrack")


def is_pointee_tag(tag: str) -> bool:
    """Elements whose Id lives in the set-wide pointee space (see NextPointeeId).

    Automation/modulation targets and macro/controller targets are referenced by
    EnvelopeTarget/PointeeId, so their ids must be unique across the whole set.
    """
    return tag.endswith("Target") or tag == "Pointee" or tag.startswith("ControllerTargets.")


# --- serialization ---------------------------------------------------------

def _esc_text(s: str) -> str:
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _attr(v: str) -> str:
    v = v.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
    # Live switches to single quotes when the value contains a double quote.
    if '"' in v and "'" not in v:
        return "'" + v + "'"
    return '"' + v.replace('"', "&quot;") + '"'


def _serialize(e: ET.Element, out: list[str]) -> None:
    out.append("<" + e.tag)
    for k, v in e.attrib.items():
        out.append(" " + k + "=" + _attr(v))
    if len(e) == 0 and not e.text:
        out.append(" />")
    else:
        out.append(">")
        if e.text:
            out.append(_esc_text(e.text))
        for c in e:
            _serialize(c, out)
        out.append("</" + e.tag + ">")
    if e.tail:
        out.append(_esc_text(e.tail))


def to_xml_bytes(root: ET.Element) -> bytes:
    out = [XML_DECL]
    _serialize(root, out)
    out.append("\n")
    return "".join(out).replace("\n", "\r\n").encode("utf-8")


# --- model -----------------------------------------------------------------

def _val(e: ET.Element | None, path: str, default: str = "") -> str:
    if e is None:
        return default
    n = e.find(path)
    return n.get("Value", default) if n is not None else default


@dataclass
class Clip:
    kind: str  # "MidiClip" | "AudioClip"
    name: str
    start: float
    end: float
    location: str  # "arrangement" | "session[<slot>]"
    elem: ET.Element = field(repr=False, compare=False, default=None)


@dataclass
class SampleRef:
    path: str
    relative_path: str
    relative_path_type: str
    pack: str
    file_size: str
    crc: str


class Track:
    def __init__(self, elem: ET.Element):
        self.elem = elem

    @property
    def kind(self) -> str:
        return self.elem.tag

    @property
    def id(self) -> str:
        return self.elem.get("Id", "")

    @property
    def name(self) -> str:
        return _val(self.elem, "Name/EffectiveName")

    @property
    def group_id(self) -> str:
        return _val(self.elem, "TrackGroupId", "-1")

    def devices(self) -> list[ET.Element]:
        devs = self.elem.find("DeviceChain/DeviceChain/Devices")
        return list(devs) if devs is not None else []

    def device_names(self) -> list[str]:
        return [device_label(d) for d in self.devices()]

    def clips(self) -> list[Clip]:
        result: list[Clip] = []
        seq = self.elem.find("DeviceChain/MainSequencer")
        if seq is None:
            return result
        for events_path in ("ClipTimeable/ArrangerAutomation/Events", "Sample/ArrangerAutomation/Events"):
            events = seq.find(events_path)
            if events is not None:
                result.extend(_clip(c, "arrangement") for c in events if c.tag in ("MidiClip", "AudioClip"))
        slots = seq.find("ClipSlotList")
        if slots is not None:
            for i, slot in enumerate(slots):
                value = slot.find("ClipSlot/Value")
                if value is not None:
                    result.extend(_clip(c, f"session[{i}]") for c in value if c.tag in ("MidiClip", "AudioClip"))
        return result


def _clip(c: ET.Element, location: str) -> Clip:
    return Clip(
        kind=c.tag,
        name=_val(c, "Name"),
        start=float(_val(c, "CurrentStart", "0")),
        end=float(_val(c, "CurrentEnd", "0")),
        location=location,
        elem=c,
    )


def device_label(d: ET.Element) -> str:
    if d.tag == "PluginDevice":
        info = d.find("PluginDesc")
        plugin = next(iter(info), None) if info is not None else None
        name = _val(plugin, "Name") or _val(plugin, "PlugName") or "?"
        fmt = plugin.tag.replace("PluginInfo", "") if plugin is not None else "Plugin"
        return f"{name} ({fmt})"
    user_name = _val(d, "UserName")
    return f"{d.tag} \"{user_name}\"" if user_name else d.tag


class LiveSet:
    def __init__(self, root: ET.Element, path: Path | None = None):
        self.root = root
        self.path = path

    # --- IO ---
    @classmethod
    def load(cls, path: str | Path) -> "LiveSet":
        path = Path(path)
        with gzip.open(path, "rb") as f:
            return cls(ET.fromstring(f.read()), path)

    @classmethod
    def from_xml_bytes(cls, data: bytes) -> "LiveSet":
        return cls(ET.fromstring(data))

    def to_xml_bytes(self) -> bytes:
        return to_xml_bytes(self.root)

    def save(self, path: str | Path) -> None:
        # mtime=0 keeps output deterministic for identical content.
        with open(path, "wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", mtime=0) as f:
            f.write(self.to_xml_bytes())

    # --- accessors ---
    @property
    def liveset(self) -> ET.Element:
        return self.root.find("LiveSet")

    @property
    def creator(self) -> str:
        return self.root.get("Creator", "")

    @property
    def tempo(self) -> str:
        return _val(self.liveset, "MainTrack/DeviceChain/Mixer/Tempo/Manual")

    @property
    def next_pointee_id(self) -> int:
        return int(_val(self.liveset, "NextPointeeId", "0"))

    @next_pointee_id.setter
    def next_pointee_id(self, value: int) -> None:
        self.liveset.find("NextPointeeId").set("Value", str(value))

    @property
    def tracks_elem(self) -> ET.Element:
        return self.liveset.find("Tracks")

    def tracks(self) -> list[Track]:
        return [Track(e) for e in self.tracks_elem if e.tag in TRACK_TAGS]

    def track_by_id(self) -> dict[str, Track]:
        return {t.id: t for t in self.tracks()}

    def scene_count(self) -> int:
        return len(self.liveset.find("Scenes"))

    def pointee_elements(self) -> Iterator[ET.Element]:
        for e in self.root.iter():
            if "Id" in e.attrib and is_pointee_tag(e.tag):
                yield e

    def sample_refs(self) -> list[SampleRef]:
        refs = []
        for sr in self.root.iter("SampleRef"):
            fr = sr.find("FileRef")
            if fr is None:
                continue
            refs.append(SampleRef(
                path=_val(fr, "Path"),
                relative_path=_val(fr, "RelativePath"),
                relative_path_type=_val(fr, "RelativePathType"),
                pack=_val(fr, "LivePackName"),
                file_size=_val(fr, "OriginalFileSize"),
                crc=_val(fr, "OriginalCrc"),
            ))
        return refs

    def plugins(self) -> list[str]:
        return sorted({device_label(d) for d in self.root.iter("PluginDevice")})
