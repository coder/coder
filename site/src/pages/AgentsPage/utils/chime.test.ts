import { getDefaultStore } from "jotai";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { chimeEnabledAtom } from "../atoms";
import { _resetForTesting, LOCK_HOLD_MS, maybePlayChime } from "./chime";

class MockLockManager {
	private held = new Set<string>();

	async request(
		name: string,
		options: LockOptions,
		callback: (lock: Lock | null) => Promise<void>,
	): Promise<void> {
		if (options.ifAvailable && this.held.has(name)) {
			await callback(null);
			return;
		}
		this.held.add(name);
		try {
			await callback({ name, mode: "exclusive" });
		} finally {
			this.held.delete(name);
		}
	}
}

describe("maybePlayChime", () => {
	let playSpy: ReturnType<typeof vi.fn>;
	let mockLocks: MockLockManager;

	beforeEach(() => {
		vi.useFakeTimers();
		localStorage.clear();
		_resetForTesting();
		getDefaultStore().set(chimeEnabledAtom, true);

		mockLocks = new MockLockManager();
		vi.stubGlobal("navigator", { locks: mockLocks });

		playSpy = vi
			.spyOn(HTMLMediaElement.prototype, "play")
			.mockResolvedValue(undefined);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	async function triggerAndSettle(
		prev: string | undefined,
		next: string,
		chatID: string,
		activeChatID: string | undefined,
	): Promise<void> {
		maybePlayChime(prev, next, chatID, activeChatID);
		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
	}

	it("chimes on running → waiting when viewing a different chat", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on running → waiting when tab is hidden", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-1");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("does not chime when viewing the finishing chat on a visible tab", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-1");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does not chime when the preference is disabled", async () => {
		getDefaultStore().set(chimeEnabledAtom, false);
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does not chime for another status transition", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "error", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it.each([undefined, "error", "interrupting", "waiting"])(
		"does not chime on %s to waiting",
		async (previousStatus) => {
			vi.spyOn(document, "hidden", "get").mockReturnValue(true);
			await triggerAndSettle(previousStatus, "waiting", "chat-1", "chat-2");
			expect(playSpy).not.toHaveBeenCalled();
		},
	);

	it("uses independent locks for different chats", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", undefined);
		maybePlayChime("running", "waiting", "chat-2", undefined);
		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
		expect(playSpy).toHaveBeenCalledTimes(2);
	});

	it("blocks duplicate chimes for the same chat while a lock is held", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		maybePlayChime("running", "waiting", "chat-1", "chat-2");

		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("falls back to immediate play when navigator.locks is unavailable", () => {
		vi.stubGlobal("navigator", { locks: undefined });

		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});
});
