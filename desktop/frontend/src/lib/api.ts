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
export function isConnectionCode(s: string): boolean {
  return s.trim().startsWith("dawgit-s3:");
}
