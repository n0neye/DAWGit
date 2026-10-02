// The contract between the file panel and the viewers.
//
// The panel decides WHAT is looked at (see viewmode.svelte.ts): one state of
// the file, or two to compare. A viewer decides HOW a kind of file is shown
// (a waveform, a picture, a Live Set's tracks, text) and keeps its own
// settings (side by side or slider, Arrangement or Session, whole file);
// it never changes what is looked at.
import type { Component } from "svelte";
import type { ProjectFile } from "../api";

// One state of a file: where it was then, and in which version ("" for the
// project folder now).
export type Side = { path: string; version: string; label: string };

export type ViewerProps = {
  root: string;
  file: ProjectFile;
  a: Side | null;   // the state looked at (null: the file is gone in it)
  b: Side | null;   // the state it is compared with (null: none, e.g. a new file)
  compare: boolean; // show the differences from b (false: just a)
  stamp: number;    // changes when the project folder may have changed: reload "now"
};

export type Viewer = {
  id: string;
  matches: (f: ProjectFile) => boolean;
  component: Component<ViewerProps>;
};
