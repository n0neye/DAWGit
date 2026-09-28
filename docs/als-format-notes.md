# .als format notes

Findings from Live 12.3.1 sets (`SampleProjects/`). Verify against other Live versions before relying on them.

## Container
- gzip-compressed UTF-8 XML, CRLF line endings, tab indentation.
- Attribute values containing `"` are written with single quotes (e.g. `ViewData`).
- Our serializer reproduces Live's output byte-for-byte for all fixtures.

## Ids
- **Track ids** (`MidiTrack/AudioTrack/GroupTrack/ReturnTrack @Id`) are stable across saves. Tracks and returns share one id space. New tracks get max+1.
- **Pointee ids**: `*Target` elements (`AutomationTarget`, `ModulationTarget`, `VolumeModulationTarget`, ...), `Pointee`, `ControllerTargets.N`. Unique set-wide, allocated from `LiveSet/NextPointeeId`, stable across saves.
  - Referenced by `EnvelopeTarget/PointeeId` (arrangement automation and clip envelopes). All references observed stay within the same track (MainTrack's tempo automation points at MainTrack's targets).
- **Positional ids**: `ClipSlot`, `TrackSendHolder`, `Scene`, `SendPreBool`, etc. are list indices.
- **Serialization counters**: `FileRef @Id`, `Vst3Preset @Id` and similar change on every save. Ignore them.

## Structure invariants
- Return tracks come after all other tracks in `LiveSet/Tracks`.
- Every track (including returns) has one `TrackSendHolder` per return track, in return-track order (by position, not id).
- `LiveSet/SendsPre` has one `SendPreBool` per return track.
- Non-return tracks have `ClipSlotList` in both `MainSequencer` and `FreezeSequencer`, one slot per scene. Return tracks have none.
- `TrackGroupId` references a `GroupTrack` id or `-1`.
- Routing targets may reference tracks as `.../Track.<id>/...`.

## Save-to-save noise (not musical changes)
`LomId`, `LomIdView`, `ViewData`, `ViewStates`, `Transport/CurrentTime`, `HighlightedTrackIndex`, `TimeSelection`, `ClipEnvelopeChooserViewState`, `LastSelected*`, `IsExpanded`, `IsFolded`, plugin window position, `Recorder/IsArmed`, `OverwriteProtectionNumber`, and the serialization counters above.

## Samples
- `SampleRef/FileRef` has `RelativePathType` (3 = project-relative, 5 = Live pack/library), `RelativePath`, absolute `Path`, `LivePackName/Id`, `OriginalFileSize`, `OriginalCrc`.

## Plugins
- `PluginDevice/PluginDesc/Vst3PluginInfo` (name, Uid). State is an opaque `Buffer`. Only parameters exposed as `PluginFloatParameter` are diffable.
