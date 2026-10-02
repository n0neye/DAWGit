// The viewers, by kind of file: the first that takes a file shows it. A new
// kind of file gets a viewer here (see types.ts for what it is given).
import type { ProjectFile } from "../api";
import type { Viewer } from "./types";
import SetViewer from "./SetViewer.svelte";
import AudioViewer from "./AudioViewer.svelte";
import VideoViewer from "./VideoViewer.svelte";
import ModelViewer from "./ModelViewer.svelte";
import ImageViewer from "./ImageViewer.svelte";
import TextViewer from "./TextViewer.svelte";

const text: Viewer = { id: "text", matches: () => true, component: TextViewer };

export const viewers: Viewer[] = [
  { id: "live-set", matches: (f) => f.kind === "set", component: SetViewer },
  { id: "audio", matches: (f) => f.kind === "audio", component: AudioViewer },
  { id: "video", matches: (f) => f.video, component: VideoViewer },
  { id: "model", matches: (f) => f.model, component: ModelViewer },
  { id: "image", matches: (f) => f.preview, component: ImageViewer },
  text, // anything else: text when it is text, a note when it isn't
];

export function viewerFor(f: ProjectFile): Viewer {
  return viewers.find((v) => v.matches(f)) ?? text;
}

export type { Side, Viewer, ViewerProps } from "./types";
