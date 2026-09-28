// Small in-app notifications (the tray app also sends Windows notifications).
export type Toast = { id: number; text: string; kind: "info" | "ok" | "warn" | "error" };

let next = 1;
export const toasts = $state<Toast[]>([]);

export function toast(text: string, kind: Toast["kind"] = "info", ms = 5000) {
  const id = next++;
  toasts.push({ id, text, kind });
  setTimeout(() => dismiss(id), ms);
}

export function dismiss(id: number) {
  const i = toasts.findIndex((t) => t.id === id);
  if (i >= 0) toasts.splice(i, 1);
}
