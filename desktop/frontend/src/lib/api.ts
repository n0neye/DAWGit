// Thin wrapper over the generated Go bindings plus small UI helpers.
import * as App from "../../bindings/dawgit/desktop/app";
export type {
  State, Version, Change, Conflict, Preview, Result, Teammate, Branch,
  Overview, TeamSummary, TeamProject,
} from "../../bindings/dawgit/desktop/models";
export type { TrackEdit } from "../../bindings/dawgit/internal/project/models";
export type { Project as ServerProject } from "../../bindings/dawgit/internal/remote/models";

export const api = App;

export function errorText(e: unknown): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "object" && e && "message" in e) return String((e as any).message);
  return String(e);
}

export function ago(iso: string): string {
  const t = Date.parse(iso);
  if (isNaN(t)) return "";
  const s = Math.max(0, (Date.now() - t) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  const d = new Date(t);
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" }) +
    " " + d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

const AUTHOR_KEY = "dawgit.author";
export function savedAuthor(): string {
  try { return localStorage.getItem(AUTHOR_KEY) ?? ""; } catch { return ""; }
}
export function rememberAuthor(name: string) {
  try { localStorage.setItem(AUTHOR_KEY, name); } catch { /* not persisted */ }
}

/** Colour class for a diff line such as "+ device Amp" or "~ clip ...". */
export function lineKind(line: string): string {
  const t = line.trimStart();
  if (t.startsWith("+")) return "add";
  if (t.startsWith("-")) return "del";
  if (t.startsWith("~")) return "mod";
  return "";
}

/** A connection code bundles storage address and keys; no token needed. */
// Overview.currentTeam when the Local projects (this computer only) are shown.
export const LOCAL = "local";

// How a long step (save, upload, download) is going; "progress" events.
export type Progress = { root: string; stage: string; done: number; total: number };

export function progressText(p: Progress, team = "the team"): string {
  const n = p.total ? ` · ${Math.min(p.done + 1, p.total)} of ${p.total}` : "";
  switch (p.stage) {
    case "scanning": return "Looking for changed files…";
    case "storing": return `Adding files to the history${n}`;
    case "uploading": return `Uploading to ${team}${n}`;
    case "downloading": return `Downloading${n}`;
    case "exporting": return `Writing the copy${n}`;
  }
  return "Working…";
}

// Short form for the sidebar.
export function progressShort(p: Progress): string {
  const n = p.total ? ` ${Math.min(p.done + 1, p.total)}/${p.total}` : "…";
  return ({ scanning: "reading files…", storing: "saving" + n, uploading: "uploading" + n,
    downloading: "downloading" + n, exporting: "exporting" + n } as Record<string, string>)[p.stage] ?? "working…";
}

export function isConnectionCode(s: string): boolean {
  return s.trim().startsWith("dawgit-s3:");
}
