/**
 * @file Shared WebSocket reconnection utility with capped exponential
 * backoff. Both the chat-list watcher (AgentsPage) and the per-chat
 * stream watcher (ChatContext) use the same reconnect-on-disconnect
 * pattern. This module extracts that logic into a single reusable
 * function so the two call sites stay in sync and the backoff math
 * lives in one place.
 *
 * @example
 * ```ts
 * const dispose = createReconnectingWebSocket({
 *   connect() {
 *     const ws = watchChats();
 *     ws.addEventListener("message", (e) => handleMessage(e));
 *     return ws;
 *   },
 *   onOpen() {
 *     console.log("connected");
 *   },
 *   onDisconnect(reconnect) {
 *     console.log(
 *       `disconnected, reconnecting in ${reconnect.delayMs}ms`,
 *     );
 *   },
 * });
 *
 * // Later, to tear down:
 * dispose();
 * ```
 */

/** Default base delay for exponential backoff (milliseconds). */
const RECONNECT_BASE_MS = 1_000;

/** Default maximum base delay cap for exponential backoff (milliseconds). */
const RECONNECT_MAX_MS = 10_000;

/** Default multiplier applied to the base delay on each retry. */
const RECONNECT_FACTOR = 2;

/**
 * Default symmetric jitter applied to the computed reconnect delay.
 * `0.3` means the final delay is randomized within ±30% of the base
 * exponential-backoff value.
 */
const RECONNECT_JITTER = 0.3;

/**
 * Metadata for the reconnect attempt that was just scheduled.
 * `attempt` is 1-based and user-facing: `1` means the first retry after
 * the connection dropped.
 */
export type ReconnectSchedule = {
	attempt: number;
	delayMs: number;
	retryingAt: string;
};

/**
 * A minimal WebSocket-like interface that the reconnection utility can
 * manage. Both native `WebSocket` and `OneWayWebSocket` satisfy this
 * contract.
 */
interface Closable {
	addEventListener(event: string, handler: (...args: unknown[]) => void): void;
	close(...args: unknown[]): void;
}

/**
 * Configuration for {@link createReconnectingWebSocket}.
 *
 * @typeParam TSocket - The concrete socket type returned by the
 *   `connect` function (e.g. `OneWayWebSocket<ServerSentEvent>`).
 */
interface ReconnectingWebSocketOptions<TSocket extends Closable> {
	/**
	 * Factory that creates and returns a new socket. Called on the
	 * initial connection and on every reconnection attempt. The caller
	 * is responsible for attaching any `message` listeners to the
	 * returned socket — this utility only manages the lifecycle
	 * (`open`, `close`, `error`) events.
	 */
	connect: () => TSocket;

	/**
	 * Called when a connection succeeds (the socket fires `open`). The
	 * backoff counter is reset before this callback runs.
	 */
	onOpen?: (socket: TSocket) => void;

	/**
	 * Called on the first disconnect after a successful connection or on a
	 * connection failure. Fires at most once per socket instance (browsers
	 * fire both `error` and `close`; only the first is forwarded). The
	 * callback receives the reconnect attempt that was just scheduled.
	 */
	onDisconnect?: (reconnect: ReconnectSchedule) => void;

	/**
	 * Decides whether a disconnect should trigger an automatic reconnect.
	 * Receives the `close` or `error` event. Defaults to always reconnecting.
	 * Return `false` to "park" the connection instead: no reconnect is
	 * scheduled and {@link onParked} fires. A parked connection can be resumed
	 * by `resumeOnVisible` or by calling `reconnect()` on the returned handle.
	 */
	shouldReconnect?: (event: Event) => boolean;

	/**
	 * Called when a disconnect is parked, i.e. {@link shouldReconnect} returned
	 * false. Fires at most once per socket instance, in place of
	 * {@link onDisconnect}.
	 */
	onParked?: (event: Event) => void;

	/**
	 * When true, a parked connection reconnects automatically when the document
	 * becomes visible or the window regains focus. Has no effect while the
	 * connection is active or a reconnect is already scheduled.
	 */
	resumeOnVisible?: boolean;

	/** Base delay in milliseconds. Defaults to {@link RECONNECT_BASE_MS}. */
	baseMs?: number;

	/**
	 * Hard upper bound on the reconnect delay in milliseconds. Jitter is
	 * applied to the capped backoff base, so the final delay never exceeds
	 * this value.
	 */
	maxMs?: number;

	/** Multiplier applied per attempt. Defaults to {@link RECONNECT_FACTOR}. */
	factor?: number;

	/**
	 * Symmetric jitter applied to the computed delay. `0.3` means the
	 * final delay may vary within ±30% of the base exponential-backoff
	 * value. Set to `0` to preserve exact legacy timing. Values are
	 * clamped to `[0, 1]`; non-finite values are treated as `0`.
	 */
	jitter?: number;

	/**
	 * Random-number source used for jitter. Defaults to `Math.random` and
	 * exists primarily as a deterministic test seam. Output is normalized
	 * to `[0, 1]`; non-finite values fall back to `0.5`.
	 */
	random?: () => number;
}

const normalizeUnitInterval = (value: number, fallback: number): number =>
	Number.isFinite(value) ? Math.min(Math.max(value, 0), 1) : fallback;

const normalizeDelayMs = (value: number, fallback: number): number =>
	Number.isFinite(value) ? Math.max(0, value) : fallback;

const applyReconnectJitter = ({
	delayMs,
	jitter,
	random,
}: {
	delayMs: number;
	jitter: number;
	random: () => number;
}): number => {
	const safeJitter = normalizeUnitInterval(jitter, 0);
	if (safeJitter <= 0) {
		return delayMs;
	}
	const safeRandom = normalizeUnitInterval(random(), 0.5);
	const jitterOffset = (safeRandom * 2 - 1) * safeJitter;
	return normalizeDelayMs(Math.round(delayMs * (1 + jitterOffset)), delayMs);
};

const getReconnectSchedule = ({
	attempt,
	baseMs,
	maxMs,
	factor,
	jitter,
	random,
}: {
	attempt: number;
	baseMs: number;
	maxMs: number;
	factor: number;
	jitter: number;
	random: () => number;
}): ReconnectSchedule => {
	const safeMaxMs = normalizeDelayMs(maxMs, 0);
	const rawDelayMs = normalizeDelayMs(
		baseMs * factor ** (attempt - 1),
		safeMaxMs,
	);
	const cappedDelayMs = Math.min(rawDelayMs, safeMaxMs);
	const jitteredDelayMs = applyReconnectJitter({
		delayMs: cappedDelayMs,
		jitter,
		random,
	});
	const delayMs = Math.min(jitteredDelayMs, safeMaxMs);
	return {
		attempt,
		delayMs,
		retryingAt: new Date(Date.now() + delayMs).toISOString(),
	};
};

/**
 * Returns true for a clean WebSocket close: the server sent a close frame
 * with a normal status (`1000`) or going-away (`1001`), or the browser
 * reported `wasClean`. Useful as a `shouldReconnect` predicate to park a
 * connection on an expected close (e.g. an idle timeout) instead of
 * reconnecting in a loop.
 */
export function isCleanClose(event: Event): boolean {
	return (
		event instanceof CloseEvent &&
		(event.wasClean || event.code === 1000 || event.code === 1001)
	);
}

/**
 * Creates a self-reconnecting WebSocket connection with capped
 * exponential backoff.
 *
 * The returned function disposes of the connection: it closes the
 * active socket (if any), cancels any pending reconnection timer, and
 * prevents further reconnection attempts. It is safe to call the
 * dispose function more than once.
 *
 * Backoff delay formula:
 * ```
 * rawDelay = baseMs * factor ^ (attempt - 1)
 * cappedDelay = min(rawDelay, maxMs)
 * jitteredDelay = round(cappedDelay * (1 + offset))
 * delay = min(jitteredDelay, maxMs)
 * offset ∈ [-jitter, +jitter]
 * ```
 *
 * The reconnect attempt counter resets after a successful `open`.
 *
 * @returns A handle with `dispose` (tears down the connection) and
 * `reconnect` (manually reopens a parked or closed connection).
 */
export interface ReconnectingWebSocketHandle {
	/**
	 * Tears down the connection: closes the active socket, cancels any pending
	 * reconnection timer, removes resume listeners, and prevents further
	 * reconnection attempts. Safe to call more than once.
	 */
	dispose: () => void;
	/**
	 * Reopens the connection immediately. Cancels any pending reconnect timer,
	 * clears the parked state, and resets the backoff counter. No-op after
	 * dispose.
	 */
	reconnect: () => void;
}

export function createReconnectingWebSocket<TSocket extends Closable>(
	options: ReconnectingWebSocketOptions<TSocket>,
): ReconnectingWebSocketHandle {
	const {
		connect: connectFn,
		onOpen,
		onDisconnect,
		shouldReconnect = () => true,
		onParked,
		resumeOnVisible = false,
		baseMs = RECONNECT_BASE_MS,
		maxMs = RECONNECT_MAX_MS,
		factor = RECONNECT_FACTOR,
		jitter = RECONNECT_JITTER,
		random = Math.random,
	} = options;

	let disposed = false;
	let parked = false;
	let lastReconnectAttempt = 0;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let activeSocket: TSocket | null = null;

	const scheduleReconnect = (schedule: ReconnectSchedule) => {
		if (disposed) {
			return;
		}
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
		}
		lastReconnectAttempt = schedule.attempt;
		reconnectTimer = setTimeout(connect, schedule.delayMs);
	};

	function connect() {
		reconnectTimer = null;
		if (disposed) {
			return;
		}
		parked = false;
		if (activeSocket) {
			activeSocket.close();
		}

		const socket = connectFn();
		activeSocket = socket;

		const handleOpen = () => {
			// Connection succeeded, reset backoff.
			lastReconnectAttempt = 0;
			onOpen?.(socket);
		};

		const handleDisconnect = (event: Event) => {
			// Guard against duplicate calls: browsers fire both "error"
			// and "close" on a failed WebSocket, so we only process the
			// first event per socket instance.
			if (activeSocket !== socket || disposed) {
				return;
			}
			activeSocket = null;

			if (!shouldReconnect(event)) {
				// Park: leave the connection closed until something resumes it
				// (resumeOnVisible or a manual reconnect call).
				parked = true;
				onParked?.(event);
				return;
			}

			const schedule = getReconnectSchedule({
				attempt: lastReconnectAttempt + 1,
				baseMs,
				maxMs,
				factor,
				jitter,
				random,
			});
			onDisconnect?.(schedule);
			scheduleReconnect(schedule);
		};

		socket.addEventListener("open", handleOpen);
		socket.addEventListener("error", (event) =>
			handleDisconnect(event as Event),
		);
		socket.addEventListener("close", (event) =>
			handleDisconnect(event as Event),
		);
	}

	const reconnect = () => {
		if (disposed) {
			return;
		}
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
		lastReconnectAttempt = 0;
		connect();
	};

	// Resume a parked connection when the tab becomes visible or focused.
	const handleResume = () => {
		if (disposed || !parked) {
			return;
		}
		if (
			typeof document !== "undefined" &&
			document.visibilityState !== "visible"
		) {
			return;
		}
		reconnect();
	};
	const resumeEnabled = resumeOnVisible && typeof window !== "undefined";
	if (resumeEnabled) {
		window.addEventListener("focus", handleResume);
		document.addEventListener("visibilitychange", handleResume);
	}

	// Kick off the first connection.
	connect();

	const dispose = () => {
		disposed = true;
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
		}
		if (resumeEnabled) {
			window.removeEventListener("focus", handleResume);
			document.removeEventListener("visibilitychange", handleResume);
		}
		if (activeSocket) {
			activeSocket.close();
		}
	};

	return { dispose, reconnect };
}
