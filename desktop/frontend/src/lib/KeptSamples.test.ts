import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";

// The Go bindings, mocked: what SampleSpots finds decides the dialog.
const api = vi.hoisted(() => ({
  SampleSpots: vi.fn(),
  BringSamplesIn: vi.fn(),
}));
vi.mock("./api", () => ({ api, errorText: (e: unknown) => String(e) }));

import KeptSamples from "./KeptSamples.svelte";

beforeEach(() => vi.clearAllMocks());

describe("KeptSamples", () => {
  it("goes straight on when no sample is only in .dawgit", async () => {
    api.SampleSpots.mockResolvedValue([{ kept: false }]);
    const onproceed = vi.fn();
    render(KeptSamples, { roots: ["C:/Song Project"], onproceed, oncancel: vi.fn() });
    await waitFor(() => expect(onproceed).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers to put teammates' samples in, then goes on", async () => {
    api.SampleSpots.mockResolvedValue([{ kept: true }, { kept: true }, { kept: false }]);
    api.BringSamplesIn.mockResolvedValue({});
    const onproceed = vi.fn();
    render(KeptSamples, { roots: ["C:/Song Project"], onproceed, oncancel: vi.fn() });
    const button = await screen.findByText("Put them in and continue");
    expect(screen.getByText(/2 samples the sets use/)).toBeTruthy();
    expect(onproceed).not.toHaveBeenCalled();
    await fireEvent.click(button);
    await waitFor(() => expect(onproceed).toHaveBeenCalledTimes(1));
    expect(api.BringSamplesIn).toHaveBeenCalledWith("C:/Song Project", false, true, false);
  });

  it("stops when a set is open in Live", async () => {
    api.SampleSpots.mockResolvedValue([{ kept: true }]);
    api.BringSamplesIn.mockResolvedValue({ liveRunning: true, openSet: "Song.als" });
    const onproceed = vi.fn();
    render(KeptSamples, { roots: ["C:/Song Project"], onproceed, oncancel: vi.fn() });
    await fireEvent.click(await screen.findByText("Put them in and continue"));
    await screen.findByText(/“Song.als” is open in Ableton Live/);
    expect(onproceed).not.toHaveBeenCalled();
  });
});
