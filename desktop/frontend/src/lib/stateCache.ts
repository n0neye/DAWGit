import type { State } from "./api";

// The last state of each project this session, so switching back to one shows
// it at once while it is read again.
const states = new Map<string, State>();

export const cachedState = (root: string) => states.get(root) ?? null;
export const rememberState = (s: State) => { states.set(s.root, s); };
