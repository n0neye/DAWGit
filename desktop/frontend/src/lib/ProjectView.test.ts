import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/svelte";

// ProjectView with the Go bindings mocked: every binding is a mock that
// resolves to null unless a test says otherwise. These tests pin what the
// page does (which binding it calls, with what, and what it says), so it
// can be split into smaller components without changing it.
const mocks = vi.hoisted(() => {
  const fns: Record<string, ReturnType<typeof vi.fn>> = {};
  const api = new Proxy(fns, { get: (o, k: string) => (o[k] ??= vi.fn(async () => null)) });
  return { fns, api, toast: vi.fn() };
});
const { api, toast } = mocks;
vi.mock("./api", async (orig) => ({ ...(await orig<typeof import("./api")>()), api: mocks.api }));
vi.mock("./notify.svelte", () => ({ toast: mocks.toast }));
vi.mock("@wailsio/runtime", async (orig) => ({ ...(await orig<typeof import("@wailsio/runtime")>()),
  Events: { On: () => () => {} } }));

import ProjectView from "./ProjectView.svelte";

// jsdom has no ResizeObserver (bind:clientHeight uses it).
globalThis.ResizeObserver ??= class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver;

let n = 0;
let ROOT = "";
beforeEach(() => {
  for (const k of Object.keys(mocks.fns)) delete mocks.fns[k];
  toast.mockClear();
  ROOT = `C:/Projects/P${++n}`; // the state cache is per root
  api.SampleSpots.mockResolvedValue([]);
  api.CommitWarnings.mockResolvedValue([]);
});
afterEach(() => cleanup());

const version = (id: string, message: string, over: Record<string, unknown> = {}) => ({
  id, short: id.slice(0, 7), author: "Alex", time: "2026-10-05T10:00:00Z", message, parents: ["p"],
  branches: [], authorId: "", inBranch: true, notHere: false, ...over,
});
const change = (path: string, status = "modified") =>
  ({ path, status, from: "", edited: false, details: [], weight: "", tracks: [] });
const state = (over: Record<string, unknown> = {}) => ({
  rules: { applied: [], fromFile: false, error: "", suggestions: [] },
  root: ROOT, name: "Song", author: "Yi", branch: "main", remoteUrl: "s3+https://storage/team",
  teamId: "t", teamName: "Band", teamChecked: true, unshared: false, online: true, offline: "",
  liveRunning: false, head: "h1", tool: "Ableton Live", openable: ["Song.als"], olderVersion: null,
  latest: "h1", unfinished: null, cloudFolder: "", changes: [] as ReturnType<typeof change>[], myEdits: [], incoming: [], takenBack: [],
  history: [version("h1", "v1", { author: "Yi", parents: [] })],
  branches: [{ name: "main", current: true, latest: null }, { name: "idea", current: false, latest: null }],
  ...over,
});
const result = (action: string, over: Record<string, unknown> = {}) => ({
  action, log: [], relinked: [], conflicts: [], liveRunning: false, openSet: "", keptWork: false, takenBack: [], ...over,
});

async function show(over: Record<string, unknown> = {}) {
  const st = state(over);
  api.State.mockResolvedValue(st);
  // The Changes list shows the changed files.
  api.ProjectFiles.mockResolvedValue(st.changes.map((c) => ({ path: c.path, status: c.status, size: 1, kind: "other",
    live: "", from: "", edited: false, preview: false, video: false, model: false })));
  const onchanged = vi.fn();
  render(ProjectView, { root: ROOT, refreshKey: 0, teams: [], onchanged, onsettings: vi.fn() });
  await screen.findByRole("heading", { name: "Song" });
  return { onchanged };
}

async function typeMessage(text: string) {
  const box = screen.getByPlaceholderText(/What did you change/);
  await fireEvent.input(box, { target: { value: text } });
}
const commitButton = () => screen.getByRole("button", { name: /^Commit/ });
const toasted = (text: string | RegExp) =>
  waitFor(() => expect(toast.mock.calls.some(([t]) => (typeof text === "string" ? t === text : text.test(t)))).toBe(true));

describe("ProjectView: committing", () => {
  it("commits all the changes and shares them", async () => {
    await show({ changes: [change("Song.als"), change("Samples/kick.wav", "added")] });
    api.Save.mockResolvedValue(result("published"));
    await typeMessage("New chorus");
    expect(commitButton().textContent).toContain("Commit & Share");
    await fireEvent.click(commitButton());
    await waitFor(() => expect(api.Save).toHaveBeenCalledWith(ROOT, "New chorus", false, {}, false, []));
    await toasted("Version committed and shared with the team");
  });

  it("commits only the ticked changes", async () => {
    await show({ changes: [change("Song.als"), change("notes.txt", "added")] });
    api.Save.mockResolvedValue(result("published"));
    const row = (await screen.findByTitle("notes.txt")).closest("li")!;
    await fireEvent.click(within(row).getByTitle("Commit this change"));
    await typeMessage("Only the set");
    expect(commitButton().textContent).toContain("Commit 1 of 2 & Share");
    await fireEvent.click(commitButton());
    await waitFor(() => expect(api.Save).toHaveBeenCalledWith(ROOT, "Only the set", false, {}, false, ["Song.als"]));
  });

  it("says what the project's checks warn about first", async () => {
    await show({ changes: [change("Assets/a.png", "added")] });
    api.CommitWarnings.mockResolvedValue(["Assets/a.png has no .meta yet"]);
    api.Save.mockResolvedValue(result("published"));
    await typeMessage("Art");
    await fireEvent.click(commitButton());
    await screen.findByText(/Assets\/a.png has no .meta yet/);
    expect(api.Save).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole("button", { name: "Commit anyway" }));
    await waitFor(() => expect(api.Save).toHaveBeenCalledTimes(1));
  });

  it("asks to close the set in Live, then tries again", async () => {
    await show({ changes: [change("Song.als")] });
    api.Save.mockResolvedValueOnce(result("", { liveRunning: true, openSet: "Song.als" }))
      .mockResolvedValueOnce(result("published"));
    await typeMessage("Mix");
    await fireEvent.click(commitButton());
    await screen.findByText("“Song.als” is open in Live");
    await fireEvent.click(screen.getByRole("button", { name: "I closed it — continue" }));
    await waitFor(() => expect(api.Save).toHaveBeenCalledTimes(2));
    await toasted("Version committed and shared with the team");
  });

  it("asks about conflicts and tries again with the decisions", async () => {
    await show({ changes: [change("Song.als")] });
    api.Save.mockResolvedValueOnce(result("", {
      conflicts: [{ key: "k1", file: "Song.als", unit: "Bass", description: "both changed it", canKeepBoth: false }],
    })).mockResolvedValueOnce(result("published"));
    await typeMessage("Bass");
    await fireEvent.click(commitButton());
    await screen.findByText("You and the team changed the same things");
    await fireEvent.click(screen.getAllByRole("button", { name: "Keep mine" })[0]);
    await fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(api.Save).toHaveBeenLastCalledWith(ROOT, "Bass", false, { k1: "ours" }, false, []));
  });

  it("shows what the team committed in the meantime before combining", async () => {
    await show({ changes: [change("Song.als")] });
    api.Save.mockResolvedValue(result("behind"));
    api.PreviewUpdate.mockResolvedValue({ action: "merge", versions: [version("t1", "Drums")], changes: [], conflicts: [],
      message: "", base: "h1", target: "t1" });
    await typeMessage("Keys");
    await fireEvent.click(commitButton());
    await screen.findByText("New on “main”");
    expect(api.PreviewUpdate).toHaveBeenCalledWith(ROOT);
  });
});

describe("ProjectView: the team", () => {
  it("tells about teammates' new versions and gets them", async () => {
    await show({ incoming: [version("t1", "Drums")] });
    expect(screen.getByText(/shared 1 new version/)).toBeTruthy();
    api.Update.mockResolvedValue(result("fast-forward"));
    await fireEvent.click(screen.getByRole("button", { name: "Get updates" }));
    await waitFor(() => expect(api.Update).toHaveBeenCalledWith(ROOT, {}, false));
    await toasted(/^You're up to date/);
  });

  it("tells about a version a teammate took back", async () => {
    await show({ takenBack: [version("w1", "oops")] });
    expect(screen.getByText(/Alex took back “oops”/)).toBeTruthy();
    api.Update.mockResolvedValue(result("taken-back", { takenBack: [version("w1", "oops")] }));
    await fireEvent.click(screen.getByRole("button", { name: "Get updates" }));
    await toasted(/Alex took back “oops”. It's taken out of your files too/);
  });

  it("switches branch from the branch menu", async () => {
    await show();
    api.SwitchBranch.mockResolvedValue(result("moved"));
    await fireEvent.click(screen.getByRole("button", { name: /main ▾/ }));
    // "idea" is there twice: to switch to, and to merge from (after).
    await fireEvent.click(within(screen.getByRole("menu")).getAllByRole("button", { name: /^idea/ })[0]);
    await waitFor(() => expect(api.SwitchBranch).toHaveBeenCalledWith(ROOT, "idea", false));
    await toasted(/Now working on “idea”/);
  });

  it("makes a new branch", async () => {
    await show();
    await fireEvent.click(screen.getByRole("button", { name: /main ▾/ }));
    await fireEvent.click(screen.getByRole("button", { name: "New branch from here…" }));
    await fireEvent.input(screen.getByLabelText("Branch name"), { target: { value: "yi-idea" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(api.CreateBranch).toHaveBeenCalledWith(ROOT, "yi-idea"));
  });
});

describe("ProjectView: versions", () => {
  it("goes back to the latest version from an older one", async () => {
    await show({ olderVersion: version("h0", "old one") });
    expect(screen.getAllByText(/You're on an older version/).length).toBeGreaterThan(0);
    api.GoToVersion.mockResolvedValue(result("moved"));
    await fireEvent.click(screen.getAllByRole("button", { name: "Back to latest" })[0]);
    await waitFor(() => expect(api.GoToVersion).toHaveBeenCalledWith(ROOT, "latest", false, false));
  });

  it("asks about uncommitted changes before going to a version", async () => {
    await show({ changes: [change("Song.als")], history: [version("h1", "v2", { parents: ["h0"] }), version("h0", "v1", { parents: [] })] });
    await fireEvent.click(screen.getByRole("button", { name: "History" }));
    await fireEvent.click(await screen.findByRole("button", { name: "Go to" }));
    await screen.findByText("Go to an older version");
    api.GoToVersion.mockResolvedValue(result("moved"));
    await fireEvent.click(screen.getByRole("button", { name: "Discard changes" }));
    await waitFor(() => expect(api.GoToVersion).toHaveBeenCalledWith(ROOT, "h0", true, false));
  });

  it("takes back the latest version from History", async () => {
    await show({ history: [version("h1", "oops", { author: "Yi" }), version("p", "v1", { parents: [] })] });
    api.PlanUndo.mockResolvedValue({ changed: ["a.txt"], blocked: [], conflicts: [], error: "",
      takeBack: { ok: true, why: "", haveIt: [], branches: [], shared: true, featureOff: false } });
    api.TakeBackVersion.mockResolvedValue(result("taken-back"));
    await fireEvent.click(screen.getByRole("button", { name: "History" }));
    await fireEvent.click(await screen.findByRole("button", { name: "Undo commit" }));
    await screen.findByText(/Removes it from the history, yours and the team's/);
    await fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Undo commit" }));
    await waitFor(() => expect(api.TakeBackVersion).toHaveBeenCalledWith(ROOT, "h1", true));
  });
});

describe("ProjectView: discarding", () => {
  it("asks before discarding everything", async () => {
    await show({ changes: [change("Song.als"), change("notes.txt", "added")] });
    api.DiscardAll.mockResolvedValue(result("discarded"));
    await fireEvent.click(await screen.findByLabelText("Discard the ticked changes"));
    await screen.findByText("Discard all your changes?");
    expect(api.DiscardAll).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole("button", { name: "Discard all" }));
    await waitFor(() => expect(api.DiscardAll).toHaveBeenCalledWith(ROOT, false));
  });
});
