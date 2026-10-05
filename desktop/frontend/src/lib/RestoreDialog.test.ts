import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";

const api = vi.hoisted(() => ({ RestorePlan: vi.fn(), Restore: vi.fn(), ChooseFolder: vi.fn() }));
vi.mock("./api", async (orig) => ({ ...(await orig<typeof import("./api")>()), api }));
vi.mock("./notify.svelte", () => ({ toast: vi.fn() }));
vi.mock("@wailsio/runtime", async (orig) => ({ ...(await orig<typeof import("@wailsio/runtime")>()),
  Events: { On: () => () => {} } }));

import RestoreDialog from "./RestoreDialog.svelte";

afterEach(() => cleanup());

describe("RestoreDialog", () => {
  it("restores from a bucket this computer doesn't back up to", async () => {
    api.RestorePlan.mockResolvedValue({ where: "storage/vault/band-backup", team: "Band", run: "", runs: [],
      projects: [{ id: "p", name: "Song", versions: 2 }], branches: 0, files: 3, bytes: 300 });
    api.Restore.mockResolvedValue(3);
    const ondone = vi.fn();
    render(RestoreDialog, { teamId: "t1", hasBackup: false, onclose: vi.fn(), ondone });
    expect(api.RestorePlan).not.toHaveBeenCalled(); // nothing to look in yet
    await fireEvent.click(screen.getByRole("button", { name: "Bucket…" }));
    const look = screen.getByRole("button", { name: "Look in this bucket" }) as HTMLButtonElement;
    expect(look.disabled).toBe(true);
    for (const [label, value] of [["Endpoint", "https://storage"], ["Bucket", "vault"], ["Access Key ID", "k"], ["Secret Access Key", "s"]]) {
      await fireEvent.input(screen.getByLabelText(label), { target: { value } });
    }
    await fireEvent.click(look);
    const from = { folder: "", storage: { endpoint: "https://storage", bucket: "vault", folder: "dawgit-backup", region: "", accessKey: "k", secretKey: "s" } };
    await waitFor(() => expect(api.RestorePlan).toHaveBeenCalledWith("t1", from, ""));
    await screen.findByText("Song");
    await fireEvent.click(screen.getByRole("button", { name: "Restore" }));
    await waitFor(() => expect(api.Restore).toHaveBeenCalledWith("t1", from, ""));
    expect(ondone).toHaveBeenCalled();
  });
});
