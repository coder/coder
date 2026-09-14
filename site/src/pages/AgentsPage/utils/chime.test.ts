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
			await callback({ name, mode: "exclusive" } as Lock);
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
		Object.defineProperty(navigator, "locks", {
			value: mockLocks,
			writable: true,
			configurable: true,
		});

		playSpy = vi
			.spyOn(HTMLMediaElement.prototype, "play")
			.mockResolvedValue(undefined);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
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

	it("blocks duplicate chimes for the same chat while a lock is held", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		maybePlayChime("running", "waiting", "chat-1", "chat-2");

		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("falls back to immediate play when navigator.locks is unavailable", () => {
		Object.defineProperty(navigator, "locks", {
			value: undefined,
			writable: true,
			configurable: true,
		});

		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});
});
