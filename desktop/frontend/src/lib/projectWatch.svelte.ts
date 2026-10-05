import { Events } from "@wailsio/runtime";
import { api, type Progress } from "./api";

// Keeps a project's page current while it's shown: call from a component's
// setup. reload reads the project again; nothing is read while busy (an
// action under way reads it when it's done).
export function watchProject(root: () => string, busy: () => boolean, reload: () => void,
  onprogress: (p: Progress | null) => void) {
  // The team's side (new versions, branches) every minute...
  $effect(() => {
    const t = setInterval(() => { if (!busy()) reload(); }, 60000);
    return () => clearInterval(t);
  });

  // ...but a Ctrl+S in Live shows up within a couple of seconds: the set's
  // size and time are checked every second, and the changes are read once
  // the file has stopped changing.
  $effect(() => {
    const r = root();
    const settle = settler();
    const t = setInterval(async () => {
      if (busy()) return;
      try {
        if (settle(await api.Signature(r))) reload();
      } catch {
        // checked again in a second
      }
    }, 1000);
    return () => clearInterval(t);
  });

  // Other files (samples added, converted, edited elsewhere) as soon as the
  // folder watcher sees them.
  $effect(() => {
    const r = root();
    api.WatchFiles(r);
    const off = Events.On("files", (ev: { data: { root: string } }) => {
      if (ev.data.root === r && !busy()) reload();
    });
    return () => {
      off();
      api.UnwatchFiles(r);
    };
  });

  // How a long step is going ("progress" events).
  $effect(() => {
    const r = root();
    let timer: ReturnType<typeof setTimeout>;
    const off = Events.On("progress", (ev: { data: Progress }) => {
      if (ev.data.root !== r) return;
      onprogress(ev.data.stage === "done" ? null : ev.data);
      // In case the final event is missed (e.g. the window reloaded).
      clearTimeout(timer);
      timer = setTimeout(() => onprogress(null), 10 * 60000);
    });
    return () => {
      off();
      clearTimeout(timer);
    };
  });
}

// settler says when a file that changed has stopped changing: given its
// signature (size and time) every second, true once a new signature is seen
// twice running. The first signature is where it starts.
export function settler() {
  let sig = "", pending = "";
  return (s: string): boolean => {
    if (!sig || s === sig) {
      sig = s;
      pending = "";
    } else if (s !== pending) {
      pending = s;
    } else {
      sig = s;
      pending = "";
      return true;
    }
    return false;
  };
}
