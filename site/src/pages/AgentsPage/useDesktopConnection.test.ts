import { renderHook } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useDesktopConnection } from "./useDesktopConnection";

vi.mock("api/api", () => ({
	watchChatDesktop: vi.fn(),
}));

// ---- Mock RFB --------------------------------------------------------------
// vi.mock is hoisted, so the factory cannot reference module-scoped variables.
// We use vi.hoisted() to define the mock class in the hoisted scope, then
// reference it from the factory and from the tests.

interface MockRFBInstance {
	scaleViewport: boolean;
	resizeSession: boolean;
	disconnect: ReturnType<typeof vi.fn>;
	addEventListener: ReturnType<typeof vi.fn>;
	listeners: Map<string, (ev: unknown) => void>;
	simulateEvent: (type: string, detail?: unknown) => void;
}

const { FakeRFB, lastInstance } = vi.hoisted(() => {
	const ref: { current: MockRFBInstance | null } = { current: null };

	class FakeRFB {
		scaleViewport = false;
		resizeSession = true;
		disconnect = vi.fn();
		addEventListener = vi.fn((type: string, handler: (ev: unknown) => void) => {
			(this as unknown as MockRFBInstance).listeners.set(type, handler);
		});
		listeners = new Map<string, (ev: unknown) => void>();

		simulateEvent(type: string, detail?: unknown) {
			const handler = this.listeners.get(type);
			if (handler) {
				handler(
					detail ? new CustomEvent(type, { detail }) : new CustomEvent(type),
				);
			}
		}

		constructor() {
			ref.current = this as unknown as MockRFBInstance;
		}
	}

	return { FakeRFB, lastInstance: ref };
});

vi.mock("@novnc/novnc/lib/rfb", () => ({
	default: FakeRFB,
}));

import { watchChatDesktop } from "api/api";

const mockWatchChatDesktop = vi.mocked(watchChatDesktop);

// ---- helpers ---------------------------------------------------------------

function getLastRFBInstance(): MockRFBInstance {
	if (!lastInstance.current) {
		throw new Error("No RFB instance was constructed");
	}
	return lastInstance.current;
}

function createMockSocket(): WebSocket {
	return { binaryType: "arraybuffer" } as unknown as WebSocket;
}

// ---- tests -----------------------------------------------------------------

describe("useDesktopConnection", () => {
	beforeEach(() => {
		mockWatchChatDesktop.mockReset();
		mockWatchChatDesktop.mockReturnValue(createMockSocket());
		lastInstance.current = null;
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("starts in idle status and does not connect automatically", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		expect(result.current.status).toBe("idle");
		expect(result.current.hasConnected).toBe(false);
		expect(result.current.rfb).toBeNull();
		expect(mockWatchChatDesktop).not.toHaveBeenCalled();
	});

	it("does nothing when chatId is undefined and connect() is called", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: undefined }),
		);

		act(() => result.current.connect());

		expect(result.current.status).toBe("idle");
		expect(mockWatchChatDesktop).not.toHaveBeenCalled();
	});

	it("transitions to connecting then connected on connect()", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();

		expect(mockWatchChatDesktop).toHaveBeenCalledWith("chat-1");
		expect(result.current.status).toBe("connecting");
		expect(result.current.hasConnected).toBe(false);

		act(() => rfb.simulateEvent("connect"));

		expect(result.current.status).toBe("connected");
		expect(result.current.hasConnected).toBe(true);
	});

	it("sets scaleViewport and resizeSession on the RFB instance", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();

		expect(rfb.scaleViewport).toBe(true);
		expect(rfb.resizeSession).toBe(false);
	});

	it("connect() is a no-op when already connecting", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		expect(result.current.status).toBe("connecting");

		mockWatchChatDesktop.mockClear();

		act(() => result.current.connect());
		expect(mockWatchChatDesktop).not.toHaveBeenCalled();
	});

	it("connect() is a no-op when already connected", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));
		expect(result.current.status).toBe("connected");

		mockWatchChatDesktop.mockClear();

		act(() => result.current.connect());
		expect(mockWatchChatDesktop).not.toHaveBeenCalled();
	});

	it("transitions to error on securityfailure", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();
		act(() =>
			rfb.simulateEvent("securityfailure", {
				status: 1,
				reason: "auth failed",
			}),
		);

		expect(result.current.status).toBe("error");
	});

	it("reconnects with exponential backoff on disconnect", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			const rfb1 = getLastRFBInstance();
			act(() => rfb1.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Disconnect — attempt 0 → 1000ms delay.
			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(999));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
			const rfb2 = getLastRFBInstance();

			// Reconnect attempt fails (no "connect" event) but desktop
			// was previously reachable, so it retries.
			// attempt 1 → 2000ms delay.
			act(() => rfb2.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(1999));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
			const rfb3 = getLastRFBInstance();

			// attempt 2 → 4000ms delay.
			act(() => rfb3.simulateEvent("disconnect", { clean: false }));

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(3999));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("resets backoff counter after a successful reconnect", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			const rfb1 = getLastRFBInstance();
			act(() => rfb1.simulateEvent("connect"));

			// First disconnect — 1000ms backoff.
			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			act(() => vi.advanceTimersByTime(1000));
			const rfb2 = getLastRFBInstance();

			// Reconnect succeeds — counter resets after stability period.
			act(() => rfb2.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Advance past the stability period so the counter resets.
			act(() => vi.advanceTimersByTime(3000));

			// Next disconnect should use 1000ms again (not 2000ms).
			act(() => rfb2.simulateEvent("disconnect", { clean: false }));

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(999));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("caps backoff at 30 seconds", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			// Burn through attempts: 1s, 2s, 4s, 8s, 16s.
			// Reconnect attempts fail (no "connect" event) but the
			// desktop was previously reachable so backoff accumulates.
			const delays = [1000, 2000, 4000, 8000, 16000];
			for (const delay of delays) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
			}

			// Attempt 5 — should be capped at 30_000, not 32_000.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(29_999));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("disconnect() cleans up and resets to idle", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));
		expect(result.current.status).toBe("connected");

		act(() => result.current.disconnect());

		expect(rfb.disconnect).toHaveBeenCalled();
		expect(result.current.status).toBe("idle");
	});

	it("disconnect() cancels pending reconnect timers", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			const rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			// Trigger reconnect timer.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			// Manually disconnect before timer fires.
			act(() => result.current.disconnect());
			expect(result.current.status).toBe("idle");

			// Timer should be cancelled — no reconnect.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("cleans up on unmount and does not reconnect", () => {
		vi.useFakeTimers();

		try {
			const { result, unmount } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			const rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			unmount();

			expect(rfb.disconnect).toHaveBeenCalled();

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("resets state when chatId changes", () => {
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string | undefined }) =>
				useDesktopConnection({ chatId }),
			{ initialProps: { chatId: "chat-aaa" as string | undefined } },
		);

		act(() => result.current.connect());
		const rfb1 = getLastRFBInstance();
		act(() => rfb1.simulateEvent("connect"));
		expect(result.current.status).toBe("connected");
		expect(result.current.hasConnected).toBe(true);

		rerender({ chatId: "chat-bbb" });

		expect(rfb1.disconnect).toHaveBeenCalled();
		expect(result.current.status).toBe("idle");
		expect(result.current.hasConnected).toBe(false);
	});

	it("attach() appends the offscreen container to the target", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));

		const container = document.createElement("div");
		act(() => result.current.attach(container));

		expect(container.children.length).toBe(1);
		expect(container.children[0]).toBeInstanceOf(HTMLDivElement);
	});

	it("attach() moves the canvas between containers without reconnecting", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));

		const container1 = document.createElement("div");
		const container2 = document.createElement("div");

		act(() => result.current.attach(container1));
		expect(container1.children.length).toBe(1);
		const screen = container1.children[0];

		act(() => result.current.attach(container2));
		expect(container2.children.length).toBe(1);
		expect(container2.children[0]).toBe(screen);
		expect(container1.children.length).toBe(0);

		// WebSocket was only opened once.
		expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
	});

	it("does not retry when disconnect fires before connect (desktop unavailable)", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			const rfb = getLastRFBInstance();

			// Disconnect fires before connect — e.g. agent returned 424
			// because portabledesktop is not installed.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));

			expect(result.current.status).toBe("error");

			// No reconnect timer should fire.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("retries when reconnect attempt fails but desktop was previously reachable", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			// Establish a successful connection first.
			act(() => result.current.connect());
			const rfb1 = getLastRFBInstance();
			act(() => rfb1.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Drop the connection — triggers a reconnect timer.
			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			// Timer fires and a new RFB is created.
			act(() => vi.advanceTimersByTime(1000));
			const rfb2 = getLastRFBInstance();

			// Reconnect attempt fails (disconnect before connect), but
			// desktop was previously reachable — should keep retrying.
			act(() => rfb2.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			// Another retry should be scheduled.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(2000));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("attach() is a no-op when the screen is already in the container", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		act(() => result.current.connect());
		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));

		const container = document.createElement("div");
		act(() => result.current.attach(container));
		expect(container.children.length).toBe(1);

		act(() => result.current.attach(container));
		expect(container.children.length).toBe(1);
	});

	it("gives up after 10 consecutive reconnect failures", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Burn through 10 reconnect attempts that all fail before
			// the handshake completes.
			for (let i = 0; i < 10; i++) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				const delay = Math.min(1000 * 2 ** i, 30_000);
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
			}

			// The 11th disconnect should give up.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("error");

			// No more retries.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("resets attempt counter on successful connect so cap does not carry over", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			// 9 failed reconnects (just under the cap).
			for (let i = 0; i < 9; i++) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				const delay = Math.min(1000 * 2 ** i, 30_000);
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
			}

			// This reconnect succeeds — counter resets after stability period.
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Advance past the stability period so the counter resets.
			act(() => vi.advanceTimersByTime(3000));

			// Another drop + failed reconnect should NOT hit the cap
			// because the counter was reset.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			act(() => vi.advanceTimersByTime(1000));
			rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");
		} finally {
			vi.useRealTimers();
		}
	});

	it("does not reset reconnect counter when connection drops before stability period", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Drop the connection immediately — before the 3s
			// stability window elapses. The counter should NOT
			// reset, preventing an infinite 1s reconnect loop.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			// First retry at 1000ms (attempt 0).
			act(() => vi.advanceTimersByTime(1000));
			rfb = getLastRFBInstance();

			// This reconnect also "succeeds" briefly then drops.
			act(() => rfb.simulateEvent("connect"));
			act(() => rfb.simulateEvent("disconnect", { clean: false }));

			// The next retry should use 2000ms (attempt 1), NOT
			// 1000ms. If the counter had been reset on connect,
			// this would be 1000ms, creating an infinite loop.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(1999));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("gives up when connection keeps flapping (connect then immediate disconnect)", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			act(() => result.current.connect());
			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Simulate 10 flapping cycles: each reconnect attempt
			// briefly connects then immediately disconnects. Since
			// the stability timer never fires, the attempt counter
			// keeps incrementing.
			for (let i = 0; i < 10; i++) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				const delay = Math.min(1000 * 2 ** i, 30_000);
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
				// Brief connect then immediate disconnect.
				act(() => rfb.simulateEvent("connect"));
			}

			// The 11th disconnect should give up.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("error");

			// No more retries.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});
});
