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

const deterministicRandom = () => 0.5;

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
			random: deterministicRandom,
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

	it("applies the minimum jitter bound when random returns 0", () => {
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
			jitter: 0.3,
			random: () => 0,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 700 });

		vi.advanceTimersByTime(699);
		expect(connect).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(2);
	});

	it("applies the maximum jitter bound when random returns 1", () => {
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
			jitter: 0.3,
			random: () => 1,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 1300 });

		vi.advanceTimersByTime(1299);
		expect(connect).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(2);
	});

	it("preserves legacy exact timing when jitter is disabled", () => {
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
			jitter: 0,
			random: () => 1,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 1000 });

		vi.advanceTimersByTime(999);
		expect(connect).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(2);
	});

	it("clamps jitter greater than 1 before delay math", () => {
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
			jitter: 1.5,
			random: () => 1,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 2000 });
	});

	it("treats negative jitter as no jitter", () => {
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
			jitter: -0.5,
			random: () => 0,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 1000 });
	});

	it("treats NaN jitter as no jitter", () => {
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
			jitter: Number.NaN,
			random: () => 0.5,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 1000 });
	});

	it("treats NaN from random() as the midpoint with no jitter offset", () => {
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
			jitter: 0.3,
			random: () => Number.NaN,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 1000 });
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
			random: () => 1,
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

	it("preserves jitter spread after raw backoff exceeds maxMs", () => {
		const makeReconnect = (random: () => number) => {
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
				jitter: 0.3,
				random,
			});

			for (let i = 0; i < 4; i++) {
				activeSocket.emit("close");
				vi.runOnlyPendingTimers();
			}
			activeSocket.emit("close");

			return disconnects[4]!;
		};

		const minReconnect = makeReconnect(() => 0);
		expectReconnectSchedule(minReconnect, { attempt: 5, delayMs: 7000 });

		vi.clearAllTimers();
		vi.setSystemTime(new Date("2025-01-01T00:00:00.000Z"));

		const maxReconnect = makeReconnect(() => 1);
		expectReconnectSchedule(maxReconnect, { attempt: 5, delayMs: 10000 });
		expect(maxReconnect.reconnect.delayMs).toBeGreaterThan(
			minReconnect.reconnect.delayMs,
		);
	});

	it("saturates overflowed backoff at maxMs instead of 0ms", () => {
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
			maxMs: 5000,
			factor: 1e308,
			jitter: 0,
			random: deterministicRandom,
		});

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[0]!, { attempt: 1, delayMs: 1000 });
		vi.runOnlyPendingTimers();

		activeSocket.emit("close");
		expectReconnectSchedule(disconnects[1]!, { attempt: 2, delayMs: 5000 });
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
			random: deterministicRandom,
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

	it("uses default backoff values when backoff options are omitted", () => {
		let activeSocket = createMockSocket();
		const connect = vi.fn(() => {
			activeSocket = createMockSocket();
			return activeSocket;
		});

		createReconnectingWebSocket({
			connect,
			random: deterministicRandom,
		});

		// Default: baseMs=1000, factor=2, maxMs=10000.
		activeSocket.emit("close");
		vi.advanceTimersByTime(999);
		expect(connect).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenCalledTimes(2);
	});
});
