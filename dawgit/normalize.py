"""Content fingerprints that ignore save-to-save noise.

Live rewrites some values on every save even when nothing musical changed:
serialization counters (FileRef/Vst3Preset Id), LOM handles, playhead position,
selection and view state. Fingerprints skip those so diff/merge only react to
real edits.
"""

from __future__ import annotations

import hashlib
import xml.etree.ElementTree as ET

from .als import TRACK_TAGS

# Elements whose whole subtree is UI/view state.
NOISE_ELEMENTS = frozenset({
    "LomId",
    "LomIdView",
    "IsContentSelectedInDocument",
    "ViewData",
    "ViewStates",
    "CurrentTime",
    "HighlightedTrackIndex",
    "TimeSelection",
    "ClipEnvelopeChooserViewState",
    "LastSelectedTimeableIndex",
    "LastSelectedClipEnvelopeIndex",
    "ScrollerTimePreserver",
    "SessionScrollPos",
    "SequencerNavigator",
    "SelectedDevice",
    "SelectedEnvelope",
    "IsExpanded",
    "BreakoutIsExpanded",
    "IsFolded",
    "ViewStateSessionTrackWidth",
    "WinPosX",
    "WinPosY",
    "OverwriteProtectionNumber",
    "IsArmed",  # record arm is per-session operator state
})

# Attributes that are view state.
NOISE_ATTRS = frozenset({"SelectedToolPanel", "SelectedTransformationName", "SelectedGeneratorName"})


def _keep_id(tag: str) -> bool:
    # Only track ids carry identity we care about; every other Id is either a
    # positional index, a serialization counter, or a pointee id (identity
    # without musical meaning).
    return tag in TRACK_TAGS


def _feed(e: ET.Element, h, skip: frozenset[str] = frozenset()) -> None:
    if e.tag in NOISE_ELEMENTS or e.tag in skip:
        return
    h.update(b"<" + e.tag.encode())
    for k in sorted(e.attrib):
        if k in NOISE_ATTRS or (k == "Id" and not _keep_id(e.tag)):
            continue
        h.update(b" " + k.encode() + b"=" + e.attrib[k].encode())
    h.update(b">")
    text = (e.text or "").strip()
    if text:
        h.update(text.encode())
    for c in e:
        _feed(c, h, skip)
    h.update(b"</>")


def fingerprint(e: ET.Element | None, skip: frozenset[str] = frozenset()) -> str:
    """Hash of e's musical content; subtrees whose tag is in skip are ignored."""
    if e is None:
        return "<none>"
    h = hashlib.sha1()
    _feed(e, h, skip)
    return h.hexdigest()
