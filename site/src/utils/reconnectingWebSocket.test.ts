import { createReconnectingWebSocket } from "./reconnectingWebSocket";

/**
 * Minimal mock that satisfies the {@link Closable} interface used by
 * the reconnection utility. Each instance records lifecycle listeners
 * and exposes helpers to inspect and fire those events.
 */
function createMockSocket() {
	const listeners: Record<string, Set<(...args: unknown[]) => void>> = {};
	const socket = {
		addEventListener: vi.fn(
			(event: string, handler: (...args: unknown[]) => void) => {
				if (!listeners[event]) {
					listeners[event] = new Set();
				}
				listeners[event].add(handler);
			},
		),
		removeEventListener: vi.fn(
			(event: string, handler: (...args: unknown[]) => void) => {
				listeners[event]?.delete(handler);
			},
		),
		close: vi.fn(),
		/** Fire all handlers registered for the given event type. */
		emit(event: string) {
			for (const handler of [...(listeners[event] ?? new Set())]) {
				handler();
			}
		},
		getListenerCount(event?: string) {
			if (event) {
				return listeners[event]?.size ?? 0;
			}
			return Object.values(listeners).reduce(
				(total, handlers) => total + handlers.size,
				0,
			);
		},
	};
	return socket;
}

beforeEach(() => {
	vi.useFakeTimers();
});

afterEach(() => {
	vi.useRealTimers();
});

describe("createReconnectingWebSocket", () => {
	it("calls connect immediately and wires lifecycle events", () => {
		const socket = createMockSocket();
		const connect = vi.fn(() => socket);
		const onOpen = vi.fn();

		createReconnectingWebSocket({ connect, onOpen });

		expect(connect).toHaveBeenCalledTimes(1);
		expect(socket.addEventListener).toHaveBeenCalledWith(
			"open",
			expect.any(Function),
		);
		expect(socket.addEventListener).toHaveBeenCalledWith(
			"error",
			expect.any(Function),
		);
		expect(socket.addEventListener).toHaveBeenCalledWith(
			"close",
			expect.any(Function),
		);

		// Simulate the socket opening.
		socket.emit("open");
		expect(onOpen).toHaveBeenCalledTimes(1);
		expect(onOpen).toHaveBeenCalledWith(socket);
	});

	it("reconnects with exponential backoff on disconnect", () => {
		let activeSocket = createMockSocket();
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			return activeSocket;
		});
		const onDisconnect = vi.fn();

		createReconnectingWebSocket({
			connect,
			onDisconnect,
			baseMs: 1000,
			maxMs: 10000,
			factor: 2,
		});

		expect(connect).toHaveBeenCalledTimes(1);

		// First disconnect — should schedule reconnect after 1000ms.
		activeSocket.emit("close");
		expect(onDisconnect).toHaveBeenCalledTimes(1);
		expect(onDisconnect).toHaveBeenLastCalledWith(0);

		vi.advanceTimersByTime(999);
		expect(connect).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(2);

		// Second disconnect — delay should be 2000ms.
		activeSocket.emit("close");
		expect(onDisconnect).toHaveBeenCalledTimes(2);
		expect(onDisconnect).toHaveBeenLastCalledWith(1);

		vi.advanceTimersByTime(1999);
		expect(connect).toHaveBeenCalledTimes(2);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(3);

		// Third disconnect — delay should be 4000ms.
		activeSocket.emit("close");
		vi.advanceTimersByTime(3999);
		expect(connect).toHaveBeenCalledTimes(3);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(4);
	});

	it("caps backoff delay at maxMs", () => {
		let activeSocket = createMockSocket();
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			return activeSocket;
		});

		createReconnectingWebSocket({
			connect,
			baseMs: 1000,
			maxMs: 5000,
			factor: 2,
		});

		// Disconnect enough times that the uncapped delay would
		// exceed maxMs: 1000, 2000, 4000, 8000 → capped at 5000.
		for (let i = 0; i < 3; i++) {
			activeSocket.emit("close");
			vi.runOnlyPendingTimers();
		}

		// The 4th disconnect would have delay = 1000 * 2^3 = 8000,
		// but should be capped at 5000.
		activeSocket.emit("close");
		vi.advanceTimersByTime(4999);
		expect(connect).toHaveBeenCalledTimes(4);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(5);
	});

	it("resets backoff on successful connection", () => {
		let activeSocket = createMockSocket();
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			return activeSocket;
		});

		createReconnectingWebSocket({
			connect,
			baseMs: 1000,
			maxMs: 10000,
			factor: 2,
		});

		// Disconnect twice to bump the attempt counter.
		activeSocket.emit("close");
		vi.runOnlyPendingTimers();
		activeSocket.emit("close");
		vi.runOnlyPendingTimers();

		// Now simulate a successful open — attempt should reset.
		activeSocket.emit("open");
		activeSocket.emit("close");

		// Next reconnect should use the base delay (1000ms), not
		// 4000ms.
		vi.advanceTimersByTime(999);
		expect(connect).toHaveBeenCalledTimes(3);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(4);
	});

	it("deduplicates error+close from the same socket", () => {
		let activeSocket = createMockSocket();
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			return activeSocket;
		});
		const onDisconnect = vi.fn();

		createReconnectingWebSocket({ connect, onDisconnect });

		const socketBeforeDisconnect = activeSocket;

		// Browser fires both error and close.
		socketBeforeDisconnect.emit("error");
		socketBeforeDisconnect.emit("close");

		// onDisconnect should only fire once.
		expect(onDisconnect).toHaveBeenCalledTimes(1);

		// Only one reconnect timer should be pending.
		vi.runOnlyPendingTimers();
		expect(connect).toHaveBeenCalledTimes(2);
	});

	it("does not accumulate lifecycle listeners across reconnects", () => {
		const sockets: ReturnType<typeof createMockSocket>[] = [];
		const connect = vi.fn(() => {
			const socket = createMockSocket();
			sockets.push(socket);
			return socket;
		});

		createReconnectingWebSocket({ connect });

		const firstSocket = sockets[0];
		expect(firstSocket.getListenerCount()).toBe(3);

		firstSocket.emit("close");
		expect(firstSocket.getListenerCount()).toBe(0);
		expect(vi.getTimerCount()).toBe(1);

		vi.runOnlyPendingTimers();
		const secondSocket = sockets[1];
		expect(secondSocket.getListenerCount()).toBe(3);

		secondSocket.emit("error");
		expect(secondSocket.getListenerCount()).toBe(0);

		vi.runOnlyPendingTimers();
		const thirdSocket = sockets[2];
		expect(thirdSocket.getListenerCount()).toBe(3);
		expect(firstSocket.getListenerCount()).toBe(0);
		expect(secondSocket.getListenerCount()).toBe(0);
	});

	it("dispose stops reconnection, detaches listeners, and closes the socket", () => {
		const socket = createMockSocket();
		const connect = vi.fn(() => socket);

		const dispose = createReconnectingWebSocket({ connect });
		expect(socket.getListenerCount()).toBe(3);

		dispose();

		expect(socket.close).toHaveBeenCalledTimes(1);
		expect(socket.getListenerCount()).toBe(0);

		// Simulating events after dispose should not cause errors or
		// additional connect calls.
		socket.emit("close");
		vi.runAllTimers();
		expect(connect).toHaveBeenCalledTimes(1);
	});

	it("dispose cancels pending reconnect timer", () => {
		let activeSocket = createMockSocket();
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			return activeSocket;
		});

		const dispose = createReconnectingWebSocket({ connect });

		// Trigger a disconnect so a timer is scheduled.
		activeSocket.emit("close");
		expect(connect).toHaveBeenCalledTimes(1);
		expect(vi.getTimerCount()).toBe(1);

		// Dispose before the timer fires.
		dispose();
		expect(vi.getTimerCount()).toBe(0);
		vi.runAllTimers();

		// No reconnection should have occurred.
		expect(connect).toHaveBeenCalledTimes(1);
	});

	it("dispose is safe to call multiple times", () => {
		const socket = createMockSocket();
		const connect = vi.fn(() => socket);

		const dispose = createReconnectingWebSocket({ connect });

		dispose();
		dispose();
		dispose();

		// close is idempotent on real WebSockets, so calling it
		// multiple times is harmless. The important thing is that
		// no reconnection is scheduled after the first dispose.
		expect(connect).toHaveBeenCalledTimes(1);
		expect(socket.close).toHaveBeenCalledTimes(1);
	});

	it("passes socket to onOpen callback", () => {
		const socket = createMockSocket();
		const connect = vi.fn(() => socket);
		const onOpen = vi.fn();

		createReconnectingWebSocket({ connect, onOpen });

		socket.emit("open");
		expect(onOpen).toHaveBeenCalledWith(socket);
	});

	it("uses default backoff values when none provided", () => {
		let activeSocket = createMockSocket();
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			return activeSocket;
		});

		createReconnectingWebSocket({ connect });

		// Default: baseMs=1000, factor=2, maxMs=10000.
		activeSocket.emit("close");
		vi.advanceTimersByTime(999);
		expect(connect).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(2);
	});
});
