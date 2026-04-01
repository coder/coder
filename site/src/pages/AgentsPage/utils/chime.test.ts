import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
	_resetForTesting,
	getChimeEnabled,
	getKylesophyEnabled,
	isKylesophyForced,
	KYLEOSOPHY_SOUNDS,
	LOCK_HOLD_MS,
	maybePlayChime,
	setChimeEnabled,
	setKylesophyEnabled,
} from "./chime";

// ---------------------------------------------------------------------------
// navigator.locks mock
// ---------------------------------------------------------------------------

// jsdom does not provide navigator.locks, so we supply a minimal
// in-process implementation that mirrors the real Web Locks API
// semantics used by useAgentChime: request() with ifAvailable.

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

// ---------------------------------------------------------------------------
// Preference helpers
// ---------------------------------------------------------------------------

describe("getChimeEnabled / setChimeEnabled", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("defaults to false when nothing is stored", () => {
		expect(getChimeEnabled()).toBe(false);
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
// Kyleosophy preference helpers
// ---------------------------------------------------------------------------

describe("getKylesophyEnabled / setKylesophyEnabled", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("defaults to false when nothing is stored", () => {
		expect(getKylesophyEnabled()).toBe(false);
	});

	it("returns true when stored as 'true'", () => {
		localStorage.setItem("agents.kyleosophy", "true");
		expect(getKylesophyEnabled()).toBe(true);
	});

	it("returns false when stored as 'false'", () => {
		localStorage.setItem("agents.kyleosophy", "false");
		expect(getKylesophyEnabled()).toBe(false);
	});

	it("setKylesophyEnabled persists the value", () => {
		setKylesophyEnabled(false);
		expect(localStorage.getItem("agents.kyleosophy")).toBe("false");
		expect(getKylesophyEnabled()).toBe(false);

		setKylesophyEnabled(true);
		expect(localStorage.getItem("agents.kyleosophy")).toBe("true");
		expect(getKylesophyEnabled()).toBe(true);
	});
});

// ---------------------------------------------------------------------------
// maybePlayChime
// ---------------------------------------------------------------------------

describe("maybePlayChime", () => {
	let playSpy: ReturnType<typeof vi.fn>;
	let mockLocks: MockLockManager;

	beforeEach(() => {
		vi.useFakeTimers();
		localStorage.clear();
		_resetForTesting();
		// Explicitly enable the chime — the default is now disabled.
		setChimeEnabled(true);

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

	// Helper: trigger maybePlayChime and flush the microtask
	// queue so the async navigator.locks.request() callback
	// runs, then advance past the LOCK_HOLD_MS hold period.
	async function triggerAndSettle(
		prev: string | undefined,
		next: string,
		chatID: string,
		activeChatID: string | undefined,
	): Promise<void> {
		maybePlayChime(prev, next, chatID, activeChatID);
		// Flush the microtask queue so the lock callback executes.
		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
	}

	// -- Chime SHOULD play --

	it("chimes on running → waiting when viewing a different chat", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on running → pending when viewing a different chat", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		await triggerAndSettle("running", "pending", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on pending → waiting (watchChats skips running)", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		await triggerAndSettle("pending", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on running → waiting when tab is hidden (same chat)", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-1");
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("chimes on running → waiting when tab is hidden (no active chat)", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "waiting", "chat-1", undefined);
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	// -- Chime should NOT play --

	it("does NOT chime when viewing the finishing chat on a visible tab", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(false);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-1");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime when preference is disabled", async () => {
		setChimeEnabled(false);
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on running → error", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "error", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on waiting → running (wrong direction)", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("waiting", "running", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime when previous status is undefined", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle(undefined, "waiting", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime when status has not changed", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("running", "running", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on error → waiting", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("error", "waiting", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	it("does NOT chime on pending → pending (no change)", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		await triggerAndSettle("pending", "pending", "chat-1", "chat-2");
		expect(playSpy).not.toHaveBeenCalled();
	});

	// -- Cross-tab deduplication --

	it("second tab is blocked while first tab holds the lock", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);

		// Simulate two tabs calling maybePlayChime for the same
		// chatID. The first acquires the lock; the second sees
		// ifAvailable=false and skips.
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		maybePlayChime("running", "waiting", "chat-1", "chat-2");

		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	it("different chatIDs acquire independent locks", async () => {
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);

		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		maybePlayChime("running", "waiting", "chat-3", "chat-2");

		await vi.advanceTimersByTimeAsync(LOCK_HOLD_MS + 50);
		expect(playSpy).toHaveBeenCalledTimes(2);
	});

	it("falls back to immediate play when navigator.locks is unavailable", async () => {
		// Remove the locks API to simulate an older browser.
		Object.defineProperty(navigator, "locks", {
			value: undefined,
			writable: true,
			configurable: true,
		});

		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		maybePlayChime("running", "waiting", "chat-1", "chat-2");
		// Should play immediately without needing to advance timers.
		expect(playSpy).toHaveBeenCalledTimes(1);
	});

	// -- Kyleosophy sound selection --

	it("uses a kyleosophy sound when kyleosophy is enabled", async () => {
		setKylesophyEnabled(true);
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);
		// Pin the random selection so the test is deterministic.
		vi.spyOn(Math, "random").mockReturnValue(0.5);

		const audioSpy = vi.spyOn(globalThis, "Audio" as never);

		await triggerAndSettle("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
		expect(audioSpy).toHaveBeenCalledTimes(1);

		const url = (audioSpy as unknown as ReturnType<typeof vi.fn>).mock
			.calls[0][0] as string;
		// Math.floor(0.5 * 8) = 4 → "/chime_5.mp3"
		expect(url).toBe("/chime_5.mp3");
		expect(KYLEOSOPHY_SOUNDS).toContain(url);
	});

	it("uses default chime.mp3 when kyleosophy is disabled", async () => {
		setKylesophyEnabled(false);
		vi.spyOn(document, "hidden", "get").mockReturnValue(true);

		const audioSpy = vi.spyOn(globalThis, "Audio" as never);

		await triggerAndSettle("running", "waiting", "chat-1", "chat-2");
		expect(playSpy).toHaveBeenCalledTimes(1);
		expect(audioSpy).toHaveBeenCalledTimes(1);

		const url = (audioSpy as unknown as ReturnType<typeof vi.fn>).mock
			.calls[0][0] as string;
		expect(url).toBe("/chime.mp3");
	});
});

// ---------------------------------------------------------------------------
// isKylesophyForced
// ---------------------------------------------------------------------------

describe("isKylesophyForced", () => {
	const originalLocationDescriptor = Object.getOwnPropertyDescriptor(
		globalThis,
		"location",
	);

	afterEach(() => {
		if (originalLocationDescriptor) {
			Object.defineProperty(globalThis, "location", originalLocationDescriptor);
		} else {
			// If location did not originally exist, remove the stub.
			delete (globalThis as Record<string, unknown>).location;
		}
	});

	it("returns true on dev.coder.com", () => {
		Object.defineProperty(globalThis, "location", {
			value: { hostname: "dev.coder.com" },
			writable: true,
			configurable: true,
		});
		expect(isKylesophyForced()).toBe(true);
		expect(getKylesophyEnabled()).toBe(true);
	});

	it("returns false on other hosts", () => {
		Object.defineProperty(globalThis, "location", {
			value: { hostname: "coder.example.com" },
			writable: true,
			configurable: true,
		});
		expect(isKylesophyForced()).toBe(false);
	});
});
