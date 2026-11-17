import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatStatus } from "#/api/typesGenerated";
import {
	_resetForTesting,
	LOCK_HOLD_MS,
	playChime,
	shouldPlayChime,
} from "./chime";

// jsdom has no navigator.locks. This mock implements the request() semantics
// playChime depends on, including ifAvailable.
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

const shouldChime = (
	prevStatus: ChatStatus | undefined,
	nextStatus: ChatStatus,
	overrides: { activeChatID?: string; isDocumentHidden?: boolean } = {},
) =>
	shouldPlayChime({
		prevStatus,
		nextStatus,
		chatID: "chat-1",
		activeChatID: "chat-2",
		isDocumentHidden: true,
		...overrides,
	});

describe("shouldPlayChime", () => {
	it("chimes on running → waiting when viewing a different chat", () => {
		expect(shouldChime("running", "waiting", { isDocumentHidden: false })).toBe(
			true,
		);
	});

	it("chimes on running → waiting when tab is hidden (same chat)", () => {
		expect(
			shouldChime("running", "waiting", {
				activeChatID: "chat-1",
				isDocumentHidden: true,
			}),
		).toBe(true);
	});

	it("chimes on running → waiting when tab is hidden (no active chat)", () => {
		expect(shouldChime("running", "waiting", { activeChatID: undefined })).toBe(
			true,
		);
	});

	it("chimes on running → paused (turn completed with the queue head under edit)", () => {
		expect(shouldChime("running", "paused")).toBe(true);
	});

	it("does NOT chime when viewing the finishing chat on a visible tab", () => {
		expect(
			shouldChime("running", "waiting", {
				activeChatID: "chat-1",
				isDocumentHidden: false,
			}),
		).toBe(false);
	});

	it.each<[string, ChatStatus | undefined, ChatStatus]>([
		["running → error", "running", "error"],
		["waiting → running (wrong direction)", "waiting", "running"],
		["previous status is undefined", undefined, "waiting"],
		["status has not changed", "running", "running"],
		["error → waiting", "error", "waiting"],
		[
			"interrupting → waiting (interrupted, not finished)",
			"interrupting",
			"waiting",
		],
	])("does NOT chime when %s", (_label, prev, next) => {
		expect(shouldChime(prev, next)).toBe(false);
	});
});

describe("playChime", () => {
	let playSpy: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		vi.useFakeTimers();
		_resetForTesting();
		Object.defineProperty(navigator, "locks", {
			value: new MockLockManager(),
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

	it("second tab is blocked while first tab holds the lock", async () => {
		// Same chatID from two tabs: the first takes the lock, the second gets
		// ifAvailable=false and skips.
		playChime("chat-1");
		playChime("chat-1");

		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("different chatIDs acquire independent locks", async () => {
		playChime("chat-1");
		playChime("chat-3");

		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
		expect(playSpy).toHaveBeenCalledTimes(2);
	});

	it("falls back to immediate play when navigator.locks is unavailable", () => {
		Object.defineProperty(navigator, "locks", {
			value: undefined,
			writable: true,
			configurable: true,
		});

		playChime("chat-1");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});
});
