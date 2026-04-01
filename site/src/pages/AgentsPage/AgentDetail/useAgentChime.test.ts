import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
	getChimeEnabled,
	maybePlayChime,
	setChimeEnabled,
} from "./useAgentChime";

// ---------------------------------------------------------------------------
// Preference helpers
// ---------------------------------------------------------------------------

describe("getChimeEnabled / setChimeEnabled", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("defaults to true when nothing is stored", () => {
		expect(getChimeEnabled()).toBe(true);
	});

	it("returns true when stored as 'true'", () => {
		localStorage.setItem("agents.chime-on-completion", "true");
		expect(getChimeEnabled()).toBe(true);
	});

	it("returns false when stored as 'false'", () => {
		localStorage.setItem("agents.chime-on-completion", "false");
		expect(getChimeEnabled()).toBe(false);
	});

	it("setChimeEnabled persists the value", () => {
		setChimeEnabled(false);
		expect(localStorage.getItem("agents.chime-on-completion")).toBe("false");
		expect(getChimeEnabled()).toBe(false);

		setChimeEnabled(true);
		expect(localStorage.getItem("agents.chime-on-completion")).toBe("true");
		expect(getChimeEnabled()).toBe(true);
	});
});

// ---------------------------------------------------------------------------
// maybePlayChime
// ---------------------------------------------------------------------------

describe("maybePlayChime", () => {
	let playSpy: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		localStorage.clear();
		playSpy = vi
			.spyOn(HTMLMediaElement.prototype, "play")
			.mockResolvedValue(undefined);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	// -- Chime SHOULD play --

	it("chimes on running → waiting when viewing a different chat", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on running → pending when viewing a different chat", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		maybePlayChime("running", "pending", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on pending → waiting (watchChats skips running)", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		maybePlayChime("pending", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on running → waiting when tab is hidden (same chat)", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", "chat-1");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on running → waiting when tab is hidden (no active chat)", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", undefined);
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	// -- Chime should NOT play --

	it("does NOT chime when viewing the finishing chat on a visible tab", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		maybePlayChime("running", "waiting", "chat-1", "chat-1");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime when preference is disabled", () => {
		setChimeEnabled(false);
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on running → error", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "error", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on waiting → running (wrong direction)", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("waiting", "running", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime when previous status is undefined", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime(undefined, "waiting", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime when status has not changed", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "running", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on error → waiting", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("error", "waiting", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on pending → pending (no change)", () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("pending", "pending", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});
});
