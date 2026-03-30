import { renderHook } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useDesktopConnection } from "./useDesktopConnection";

vi.mock("#/api/api", () => ({
	watchChatDesktop: vi.fn(),
}));

// ---- Mock RFB --------------------------------------------------------------
// vi.mock is hoisted, so the factory cannot reference module-scoped variables.
// We use vi.hoisted() to define the mock class in the hoisted scope, then
// reference it from the factory and from the tests.

interface MockRFBInstance {
	scaleViewport: boolean;
	resizeSession: boolean;
	clipboardPasteFrom: ReturnType<typeof vi.fn>;
	disconnect: ReturnType<typeof vi.fn>;
	sendKey: ReturnType<typeof vi.fn>;
	addEventListener: ReturnType<typeof vi.fn>;
	listeners: Map<string, (ev: unknown) => void>;
	simulateEvent: (type: string, detail?: unknown) => void;
}

const { FakeRFB, lastInstance } = vi.hoisted(() => {
	const ref: { current: MockRFBInstance | null } = { current: null };
	// When true, the constructor throws to simulate failures
	// like missing WebGL support.
	let shouldThrow = false;

	class FakeRFB implements MockRFBInstance {
		static set throwOnConstruct(v: boolean) {
			shouldThrow = v;
		}

		scaleViewport = false;
		resizeSession = true;
		clipboardPasteFrom = vi.fn();
		disconnect = vi.fn();
		sendKey = vi.fn();
		addEventListener = vi.fn((type: string, handler: (ev: unknown) => void) => {
			this.listeners.set(type, handler);
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
			if (shouldThrow) {
				throw new Error("WebGL not supported");
			}
			ref.current = this;
		}
	}
	return { FakeRFB, lastInstance: ref };
});

vi.mock("@novnc/novnc/lib/rfb", () => ({
	default: FakeRFB,
}));

import { watchChatDesktop } from "#/api/api";

const mockWatchChatDesktop = vi.mocked(watchChatDesktop);
const mockClipboardReadText = vi.fn<() => Promise<string>>();
const mockClipboardWriteText = vi.fn<(text: string) => Promise<void>>();

// ---- Mock ResizeObserver ----------------------------------------------------

interface FakeResizeObserverInstance {
	disconnect: ReturnType<typeof vi.fn>;
	simulateResize: (width: number, height: number) => void;
}

let resizeObserverInstances: FakeResizeObserverInstance[] = [];

class MockResizeObserver {
	private _callback: ResizeObserverCallback;
	private _disconnect = vi.fn();

	constructor(callback: ResizeObserverCallback) {
		this._callback = callback;
		const self = this;
		resizeObserverInstances.push({
			disconnect: this._disconnect,
			simulateResize(width: number, height: number) {
				self._callback(
					[{ contentRect: { width, height } } as ResizeObserverEntry],
					self as unknown as ResizeObserver,
				);
			},
		});
	}

	observe(_target: Element) {}
	unobserve(_target: Element) {}
	disconnect() {
		this._disconnect();
	}
}

// ---- helpers ---------------------------------------------------------------

function getLastRFBInstance(): MockRFBInstance {
	if (!lastInstance.current) {
		throw new Error("No RFB instance was constructed");
	}
	return lastInstance.current;
}

function getLastResizeObserver(): FakeResizeObserverInstance {
	const instance = resizeObserverInstances[resizeObserverInstances.length - 1];
	if (!instance) {
		throw new Error("No ResizeObserver was constructed");
	}
	return instance;
}

function createMockSocket(): WebSocket {
	const socket = new WebSocket("ws://localhost");
	vi.spyOn(socket, "close").mockImplementation(() => {});
	return socket;
}

// ---- tests -----------------------------------------------------------------

describe("useDesktopConnection", () => {
	beforeEach(() => {
		mockWatchChatDesktop.mockReset();
		mockWatchChatDesktop.mockReturnValue(createMockSocket());
		lastInstance.current = null;
		FakeRFB.throwOnConstruct = false;
		resizeObserverInstances = [];
		globalThis.ResizeObserver =
			MockResizeObserver as unknown as typeof ResizeObserver;
		mockClipboardReadText.mockReset();
		mockClipboardReadText.mockResolvedValue("");
		mockClipboardWriteText.mockReset();
		mockClipboardWriteText.mockResolvedValue();
		Object.defineProperty(navigator, "clipboard", {
			configurable: true,
			value: {
				readText: mockClipboardReadText,
				writeText: mockClipboardWriteText,
			},
		});
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("does nothing when chatId is undefined", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: undefined }),
		);

		expect(result.current.status).toBe("idle");
		expect(result.current.hasConnected).toBe(false);
		expect(result.current.rfb).toBeNull();
		expect(mockWatchChatDesktop).not.toHaveBeenCalled();
	});

	it("auto-connects on mount when chatId is set", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		expect(mockWatchChatDesktop).toHaveBeenCalledWith("chat-1");
		expect(result.current.status).toBe("connecting");

		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));

		expect(result.current.status).toBe("connected");
		expect(result.current.hasConnected).toBe(true);
	});

	it("sets scaleViewport and resizeSession on the RFB instance", () => {
		renderHook(() => useDesktopConnection({ chatId: "chat-1" }));
		const rfb = getLastRFBInstance();

		expect(rfb.scaleViewport).toBe(true);
		expect(rfb.resizeSession).toBe(false);
	});

	it("transitions to error on securityfailure", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

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

			// Reconnect succeeds then drops again.
			// attempt 1 → 2000ms delay.
			act(() => rfb2.simulateEvent("connect"));
			act(() => rfb2.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(1999));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
			const rfb3 = getLastRFBInstance();

			// attempt 2 → 4000ms delay.
			act(() => rfb3.simulateEvent("connect"));
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
			renderHook(() => useDesktopConnection({ chatId: "chat-1" }));

			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			// Burn through attempts: 1s, 2s, 4s, 8s, 16s.
			const delays = [1000, 2000, 4000, 8000, 16000];
			for (const delay of delays) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
				act(() => rfb.simulateEvent("connect"));
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

	it("cleans up on unmount and does not reconnect", () => {
		vi.useFakeTimers();

		try {
			const { unmount } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

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

	it("tears down and reconnects when chatId changes", () => {
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string | undefined }) =>
				useDesktopConnection({ chatId }),
			{ initialProps: { chatId: "chat-aaa" as string | undefined } },
		);

		const rfb1 = getLastRFBInstance();
		act(() => rfb1.simulateEvent("connect"));
		expect(result.current.status).toBe("connected");
		expect(result.current.hasConnected).toBe(true);

		mockWatchChatDesktop.mockClear();
		rerender({ chatId: "chat-bbb" });

		// Old RFB was torn down.
		expect(rfb1.disconnect).toHaveBeenCalled();
		expect(result.current.hasConnected).toBe(false);

		// Auto-reconnected with the new chatId.
		expect(mockWatchChatDesktop).toHaveBeenCalledWith("chat-bbb");
		expect(result.current.status).toBe("connecting");
	});

	it("attach() appends the offscreen container to the target", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

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

		// WebSocket was only opened once (plus the initial auto-connect).
		expect(mockWatchChatDesktop).toHaveBeenCalledTimes(1);
	});

	it("stores remote clipboard text when the server sends it", async () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));
		act(() => rfb.simulateEvent("clipboard", { text: "from remote" }));

		expect(result.current.remoteClipboardText).toBe("from remote");
		await vi.waitFor(() => {
			expect(mockClipboardWriteText).toHaveBeenCalledWith("from remote");
		});
	});

	it("does not retry when disconnect fires before connect (desktop unavailable)", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			const rfb = getLastRFBInstance();

			// Disconnect fires before connect — e.g. agent returned 424.
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

	it("does not retry when reconnect attempt fails before handshake completes", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			const rfb1 = getLastRFBInstance();
			act(() => rfb1.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			// Drop the connection — triggers a reconnect timer.
			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			// Timer fires and a new RFB is created.
			act(() => vi.advanceTimersByTime(1000));
			const rfb2 = getLastRFBInstance();

			// Reconnect attempt fails (disconnect before handshake).
			act(() => rfb2.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("error");

			// No further retry should be scheduled.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("attach() is a no-op when the screen is already in the container", () => {
		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

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

			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			for (let i = 0; i < 10; i++) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				const delay = Math.min(1000 * 2 ** i, 30_000);
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
			}

			// The 11th disconnect should give up.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("error");

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

			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			for (let i = 0; i < 9; i++) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				const delay = Math.min(1000 * 2 ** i, 30_000);
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
				act(() => rfb.simulateEvent("connect"));
			}

			expect(result.current.status).toBe("connected");

			// Advance past the stability period so the counter resets.
			act(() => vi.advanceTimersByTime(3000));

			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			act(() => vi.advanceTimersByTime(1000));
			rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
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

			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			act(() => vi.advanceTimersByTime(1000));
			rfb = getLastRFBInstance();

			act(() => rfb.simulateEvent("connect"));
			act(() => rfb.simulateEvent("disconnect", { clean: false }));

			// Should use 2000ms (attempt 1), not 1000ms.
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

			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			for (let i = 0; i < 10; i++) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				const delay = Math.min(1000 * 2 ** i, 30_000);
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
				act(() => rfb.simulateEvent("connect"));
			}

			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("error");

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("does not retry after securityfailure even if desktop was previously reachable", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			const rfb1 = getLastRFBInstance();
			act(() => rfb1.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			act(() => vi.advanceTimersByTime(1000));
			const rfb2 = getLastRFBInstance();

			act(() =>
				rfb2.simulateEvent("securityfailure", {
					status: 1,
					reason: "Insufficient resources",
				}),
			);
			act(() => rfb2.simulateEvent("disconnect", { clean: false }));

			expect(result.current.status).toBe("error");

			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	// -- Generation counter ---------------------------------------------------

	it("stale event handlers from previous session are ignored after reconnect", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			const rfb1 = getLastRFBInstance();
			act(() => rfb1.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			// Timer fires, doConnect() runs and bumps generation.
			act(() => vi.advanceTimersByTime(1000));
			expect(result.current.status).toBe("connecting");

			// Old rfb1 fires a late "disconnect" — should be ignored.
			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("connecting");
		} finally {
			vi.useRealTimers();
		}
	});

	// -- Connection timeout ---------------------------------------------------

	it("transitions to error when connection times out", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			expect(result.current.status).toBe("connecting");

			act(() => vi.advanceTimersByTime(29_999));
			expect(result.current.status).toBe("connecting");

			act(() => vi.advanceTimersByTime(1));
			expect(result.current.status).toBe("error");
		} finally {
			vi.useRealTimers();
		}
	});

	it("clears timeout when connection succeeds before deadline", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			const rfb = getLastRFBInstance();
			expect(result.current.status).toBe("connecting");

			act(() => vi.advanceTimersByTime(15_000));
			expect(result.current.status).toBe("connecting");

			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");

			act(() => vi.advanceTimersByTime(20_000));
			expect(result.current.status).toBe("connected");
		} finally {
			vi.useRealTimers();
		}
	});

	// -- Reconnect button -----------------------------------------------------

	it("reconnect() tears down and restarts the connection", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			const rfb = getLastRFBInstance();

			// Disconnect before handshake → error.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("error");

			// User clicks Reconnect.
			mockWatchChatDesktop.mockClear();
			act(() => result.current.reconnect());

			expect(mockWatchChatDesktop).toHaveBeenCalledWith("chat-1");
			expect(result.current.status).toBe("connecting");
		} finally {
			vi.useRealTimers();
		}
	});

	it("reconnect() resets the attempt counter after hitting the cap", () => {
		vi.useFakeTimers();

		try {
			const { result } = renderHook(() =>
				useDesktopConnection({ chatId: "chat-1" }),
			);

			let rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			// Exhaust all 10 reconnect attempts.
			for (let i = 0; i < 10; i++) {
				act(() => rfb.simulateEvent("disconnect", { clean: false }));
				const delay = Math.min(1000 * 2 ** i, 30_000);
				act(() => vi.advanceTimersByTime(delay));
				rfb = getLastRFBInstance();
			}

			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("error");

			// Reconnect should work — counter was reset.
			mockWatchChatDesktop.mockClear();
			act(() => result.current.reconnect());

			expect(result.current.status).toBe("connecting");
			rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));
			expect(result.current.status).toBe("connected");
		} finally {
			vi.useRealTimers();
		}
	});

	// -- chatId transitions ---------------------------------------------------

	it("resets to idle when chatId becomes undefined", () => {
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string | undefined }) =>
				useDesktopConnection({ chatId }),
			{ initialProps: { chatId: "chat-1" as string | undefined } },
		);

		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));
		expect(result.current.status).toBe("connected");

		rerender({ chatId: undefined });

		expect(rfb.disconnect).toHaveBeenCalled();
		expect(result.current.status).toBe("idle");
		expect(result.current.hasConnected).toBe(false);
	});

	it("cancels pending reconnect timer on chatId change", () => {
		vi.useFakeTimers();

		try {
			const { result, rerender } = renderHook(
				({ chatId }: { chatId: string | undefined }) =>
					useDesktopConnection({ chatId }),
				{ initialProps: { chatId: "chat-aaa" as string | undefined } },
			);

			const rfb = getLastRFBInstance();
			act(() => rfb.simulateEvent("connect"));

			// Drop → reconnect timer pending.
			act(() => rfb.simulateEvent("disconnect", { clean: false }));
			expect(result.current.status).toBe("disconnected");

			// chatId changes before timer fires.
			mockWatchChatDesktop.mockClear();
			rerender({ chatId: "chat-bbb" });

			// Should connect with new chatId, not fire old timer.
			expect(mockWatchChatDesktop).toHaveBeenCalledWith("chat-bbb");
			expect(result.current.status).toBe("connecting");

			// Old timer should be cancelled — no extra connect.
			mockWatchChatDesktop.mockClear();
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatDesktop).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	// -- Constructor failure --------------------------------------------------

	it("closes socket and sets error when RFB constructor throws", () => {
		const mockSocket = createMockSocket();
		mockWatchChatDesktop.mockReturnValue(mockSocket);
		FakeRFB.throwOnConstruct = true;

		const { result } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		expect(result.current.status).toBe("error");
		expect(mockSocket.close).toHaveBeenCalled();
	});

	// -- Stale connect event --------------------------------------------------

	it("ignores stale connect event from a previous session", () => {
		vi.useFakeTimers();

		try {
			const { result, rerender } = renderHook(
				({ chatId }: { chatId: string | undefined }) =>
					useDesktopConnection({ chatId }),
				{ initialProps: { chatId: "chat-aaa" as string | undefined } },
			);

			const rfb1 = getLastRFBInstance();
			// Don't fire connect yet — switch chatId first.

			rerender({ chatId: "chat-bbb" });
			expect(result.current.status).toBe("connecting");

			// Old rfb1 fires a late "connect" — should be ignored.
			act(() => rfb1.simulateEvent("connect"));

			// hasConnected should still be false — the connect was
			// from the stale session.
			expect(result.current.hasConnected).toBe(false);
			expect(result.current.status).toBe("connecting");
		} finally {
			vi.useRealTimers();
		}
	});

	// -- Visibility observer (ResizeObserver) ---------------------------------

	it("forces scaleViewport on hidden→visible transition", () => {
		renderHook(() => useDesktopConnection({ chatId: "chat-1" }));
		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));

		const observer = getLastResizeObserver();

		// First observation with nonzero size (initial attach).
		act(() => observer.simulateResize(800, 600));

		// Container hidden (ancestor applies display: none).
		act(() => observer.simulateResize(0, 0));

		// Reset so we can detect re-assignment.
		rfb.scaleViewport = false;

		// Container visible again — should force rescale.
		act(() => observer.simulateResize(800, 600));

		expect(rfb.scaleViewport).toBe(true);
	});

	it("does not force scaleViewport on normal nonzero→nonzero resize", () => {
		renderHook(() => useDesktopConnection({ chatId: "chat-1" }));
		const rfb = getLastRFBInstance();
		act(() => rfb.simulateEvent("connect"));

		const observer = getLastResizeObserver();

		// Initial nonzero observation.
		act(() => observer.simulateResize(800, 600));

		// Reset so we can detect re-assignment.
		rfb.scaleViewport = false;

		// Normal resize — not a hidden→visible transition.
		act(() => observer.simulateResize(1024, 768));

		expect(rfb.scaleViewport).toBe(false);
	});

	it("disconnects visibility observer on unmount", () => {
		const { unmount } = renderHook(() =>
			useDesktopConnection({ chatId: "chat-1" }),
		);

		getLastRFBInstance();
		const observer = getLastResizeObserver();

		unmount();

		expect(observer.disconnect).toHaveBeenCalled();
	});

	it("disconnects visibility observer before reconnect", () => {
		vi.useFakeTimers();

		try {
			renderHook(() => useDesktopConnection({ chatId: "chat-1" }));
			const rfb1 = getLastRFBInstance();
			act(() => rfb1.simulateEvent("connect"));

			expect(resizeObserverInstances).toHaveLength(1);
			const observer1 = resizeObserverInstances[0];

			// Trigger reconnect.
			act(() => rfb1.simulateEvent("disconnect", { clean: false }));
			act(() => vi.advanceTimersByTime(1000));

			// Old observer should be disconnected.
			expect(observer1.disconnect).toHaveBeenCalled();

			// New observer created for the new connection.
			expect(resizeObserverInstances).toHaveLength(2);
		} finally {
			vi.useRealTimers();
		}
	});

	it("ignores stale visibility observer callback after chatId change", () => {
		const { rerender } = renderHook(
			({ chatId }: { chatId: string | undefined }) =>
				useDesktopConnection({ chatId }),
			{ initialProps: { chatId: "chat-aaa" as string | undefined } },
		);

		const rfb1 = getLastRFBInstance();
		act(() => rfb1.simulateEvent("connect"));

		const observer1 = resizeObserverInstances[0];

		// Set nonzero previous dimensions on old observer.
		act(() => observer1.simulateResize(800, 600));

		// Change chatId — triggers teardown + new connection.
		rerender({ chatId: "chat-bbb" });

		const rfb2 = getLastRFBInstance();
		act(() => rfb2.simulateEvent("connect"));

		// Reset so we can detect re-assignment.
		rfb2.scaleViewport = false;

		// Fire a stale hidden→visible transition on the OLD
		// observer. The generation mismatch should prevent it
		// from writing to the current RFB instance.
		act(() => observer1.simulateResize(0, 0));
		act(() => observer1.simulateResize(800, 600));

		expect(rfb2.scaleViewport).toBe(false);
	});
});
