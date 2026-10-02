import { fileURL, previewURL } from "../api";
import type { Side } from "./types";

// A file's bytes in one state. Now's address changes with stamp, so the
// file is read again when the folder may have changed.
export function sideURL(root: string, s: Side, stamp: number): string {
  return s.version ? fileURL(root, s.path, s.version) : `${fileURL(root, s.path)}&t=${stamp}`;
}

// A design file as a picture, in one state.
export function sidePreviewURL(root: string, s: Side, stamp: number): string {
  return s.version ? previewURL(root, s.path, s.version) : `${previewURL(root, s.path)}&t=${stamp}`;
}
