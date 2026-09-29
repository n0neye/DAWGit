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
export type Progress = { root: string; stage: string; done: number; total: number; bytes?: number; totalBytes?: number };

const transfers = (p: Progress) => (p.stage === "uploading" || p.stage === "downloading") && !!p.totalBytes;

// How far along, 0..1: by bytes for transfers, else by files.
export function progressFraction(p: Progress): number {
  if (transfers(p)) return Math.min(1, (p.bytes ?? 0) / p.totalBytes!);
  return p.total ? p.done / p.total : 0;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024, i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`;
}

function formatDuration(s: number): string {
  if (s < 60) return `${Math.max(1, Math.round(s))} s`;
  if (s < 3600) return `${Math.round(s / 60)} min`;
  return `${Math.floor(s / 3600)} h ${Math.round((s % 3600) / 60)} min`;
}

// Recent (time, bytes) samples per folder and stage, for the speed.
const samples = new Map<string, { stage: string; points: [number, number][] }>();

// "120 MB of 800 MB · 5.2 MB/s · about 2 min left" for transfers, else "".
export function progressDetail(p: Progress): string {
  if (!transfers(p)) return "";
  const now = performance.now(), bytes = p.bytes ?? 0;
  let s = samples.get(p.root);
  if (!s || s.stage !== p.stage || bytes < (s.points.at(-1)?.[1] ?? 0)) {
    s = { stage: p.stage, points: [] };
    samples.set(p.root, s);
  }
  if (s.points.at(-1)?.[1] !== bytes) s.points.push([now, bytes]);
  while (s.points.length > 2 && now - s.points[0][0] > 5000) s.points.shift();
  let text = `${formatBytes(bytes)} of ${formatBytes(p.totalBytes!)}`;
  const [t0, b0] = s.points[0];
  const secs = (now - t0) / 1000;
  if (secs >= 0.7 && bytes > b0) {
    const rate = (bytes - b0) / secs;
    text += ` · ${formatBytes(rate)}/s · about ${formatDuration((p.totalBytes! - bytes) / rate)} left`;
  }
  return text;
}

export function progressText(p: Progress, team = "the team"): string {
  const files = transfers(p) || p.stage === "exporting" || p.stage === "storing" ? " files" : "";
  const n = p.total ? ` · ${Math.min(p.done + 1, p.total)} of ${p.total}${p.total === 1 ? files.replace("files", "file") : files}` : "";
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
  const n = transfers(p) ? ` ${Math.round(progressFraction(p) * 100)}%`
    : p.total ? ` ${Math.min(p.done + 1, p.total)}/${p.total}` : "…";
  return ({ scanning: "reading files…", storing: "saving" + n, uploading: "uploading" + n,
    downloading: "downloading" + n, exporting: "exporting" + n } as Record<string, string>)[p.stage] ?? "working…";
}

export function isConnectionCode(s: string): boolean {
  return s.trim().startsWith("dawgit-s3:");
}
