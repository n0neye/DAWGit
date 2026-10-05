import { Events } from "@wailsio/runtime";
import { api } from "./api";

// Big files going up to the team's storage in the background, by project
// root (see desktop/preupload.go): the icon on a project shows them.
export type Preupload = { root: string; path: string; bytes: number; total: number; done: boolean };

export const preuploads = $state<Record<string, Preupload>>({});

let watching = false;

// watchPreuploads follows them; once, from the app's start.
export function watchPreuploads() {
  if (watching) return;
  watching = true;
  api.Preuploads().then((list) => {
    for (const p of list ?? []) preuploads[p.root] = p;
  }).catch(() => {});
  Events.On("preupload", (ev: { data: Preupload }) => {
    const p = ev.data;
    if (p.done) delete preuploads[p.root];
    else preuploads[p.root] = p;
  });
}
