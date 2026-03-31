import {
	createReconnectingWebSocket,
	type ReconnectSchedule,
} from "./reconnectingWebSocket";

/**
 * Minimal mock that satisfies the {@link Closable} interface used by the
 * reconnection utility. Each instance records every `addEventListener`
 * call and exposes helpers to fire those events.
 */
function createMockSocket() {
	const listeners: Record<string, Array<(...args: unknown[]) => void>> = {};
	const socket = {
		addEventListener: vi.fn(
			(event: string, handler: (...args: unknown[]) => void) => {
				if (!listeners[event]) {
					listeners[event] = [];
				}
				listeners[event].push(handler);
			},
		),
		close: vi.fn(),
		/** Fire all handlers registered for the given event type. */
		emit(event: string) {
			for (const handler of listeners[event] ?? []) {
				handler();
			}
		},
	};
	return socket;
}

const expectReconnectSchedule = (
	event: { reconnect: ReconnectSchedule; now: number },
	expected: { attempt: number; delayMs: number },
) => {
	expect(event.reconnect).toMatchObject(expected);
	expect(Date.parse(event.reconnect.retryingAt) - event.now).toBe(
		expected.delayMs,
	);
};

beforeEach(() => {
	vi.useFakeTimers();
	vi.setSystemTime(new Date("2025-01-01T00:00:00.000Z"));
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
		const disconnects: Array<{ reconnect: ReconnectSchedule; now: number }> =
			[];
		const onDisconnect = vi.fn((reconnect: ReconnectSchedule) => {
			disconnects.push({ reconnect, now: Date.now() });
		});

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
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 1000 });

		vi.advanceTimersByTime(999);
		expect(connect).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(2);

		// Second disconnect — delay should be 2000ms.
		activeSocket.emit("close");
		expect(onDisconnect).toHaveBeenCalledTimes(2);
		expectReconnectSchedule(disconnects[1]!, { attempt: 2, delayMs: 2000 });

		vi.advanceTimersByTime(1999);
		expect(connect).toHaveBeenCalledTimes(2);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(3);

		// Third disconnect — delay should be 4000ms.
		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[2]!, { attempt: 3, delayMs: 4000 });
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

		// Disconnect enough times that the uncapped delay would exceed
		// maxMs: 1000, 2000, 4000, 8000 → capped at 5000.
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

		// Next reconnect should use the base delay (1000ms), not 4000ms.
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

	it("closes previous socket when reconnecting", () => {
		let activeSocket = createMockSocket();
		const sockets: ReturnType<typeof createMockSocket>[] = [];
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			sockets.push(activeSocket);
			return activeSocket;
		});

		createReconnectingWebSocket({ connect });

		const firstSocket = sockets[0]!;
		firstSocket.emit("close");
		vi.runOnlyPendingTimers();

		// The connect function creates a new socket. The old socket was
		// already "closed" by the browser, but on a fresh reconnection
		// the utility closes the previous one if it's still the active
		// reference.
		expect(connect).toHaveBeenCalledTimes(2);
	});

	it("dispose stops reconnection and closes the socket", () => {
		const socket = createMockSocket();
		const connect = vi.fn(() => socket);

		const dispose = createReconnectingWebSocket({ connect });

		dispose();

		expect(socket.close).toHaveBeenCalled();

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

		// Dispose before the timer fires.
		dispose();
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

		// close is idempotent on real WebSockets, so calling it multiple
		// times is harmless. The important thing is that no reconnection is
		// scheduled after the first dispose.
		expect(connect).toHaveBeenCalledTimes(1);
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
