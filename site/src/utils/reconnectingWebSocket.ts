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
 *   onDisconnect() {
 *     console.log("disconnected, will reconnect automatically");
 *   },
 * });
 *
 * // Later, to tear down:
 * dispose();
 * ```
 */

/** Default base delay for exponential backoff (milliseconds). */
const RECONNECT_BASE_MS = 1_000;

/** Default maximum delay cap for exponential backoff (milliseconds). */
const RECONNECT_MAX_MS = 10_000;

/** Default multiplier applied to the base delay on each retry. */
const RECONNECT_FACTOR = 2;

/**
 * A minimal WebSocket-like interface that the reconnection utility
 * can manage. Both native `WebSocket` and `OneWayWebSocket` satisfy
 * this contract.
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
	 * Called when a connection succeeds (the socket fires `open`).
	 * The backoff counter is reset before this callback runs.
	 */
	onOpen?: (socket: TSocket) => void;

	/**
	 * Called on the first disconnect after a successful connection or
	 * on a connection failure. Fires at most once per socket instance
	 * (browsers fire both `error` and `close`; only the first is
	 * forwarded). A reconnection is scheduled automatically after
	 * this callback returns.
	 *
	 * @param attempt - The zero-based reconnection attempt counter
	 *   *before* it is incremented for the upcoming retry. A value of
	 *   `0` means this is the first disconnect since the last
	 *   successful connection.
	 */
	onDisconnect?: (attempt: number) => void;

	/** Base delay in milliseconds. Defaults to {@link RECONNECT_BASE_MS}. */
	baseMs?: number;

	/** Maximum delay cap in milliseconds. Defaults to {@link RECONNECT_MAX_MS}. */
	maxMs?: number;

	/** Multiplier applied per attempt. Defaults to {@link RECONNECT_FACTOR}. */
	factor?: number;
}

/**
 * Creates a self-reconnecting WebSocket connection with capped
 * exponential backoff.
 *
 * The returned function disposes of the connection: it closes the
 * active socket (if any), cancels any pending reconnection timer,
 * and prevents further reconnection attempts. It is safe to call
 * the dispose function more than once.
 *
 * Backoff delay formula:
 * ```
 * delay = min(baseMs * factor ^ attempt, maxMs)
 * ```
 *
 * The attempt counter resets to `0` whenever a connection
 * successfully opens.
 *
 * @returns A dispose function that tears down the connection.
 */
export function createReconnectingWebSocket<TSocket extends Closable>(
	options: ReconnectingWebSocketOptions<TSocket>,
): () => void {
	const {
		connect: connectFn,
		onOpen,
		onDisconnect,
		baseMs = RECONNECT_BASE_MS,
		maxMs = RECONNECT_MAX_MS,
		factor = RECONNECT_FACTOR,
	} = options;

	let disposed = false;
	let reconnectAttempt = 0;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let activeSocket: TSocket | null = null;

	// Schedule a reconnect with capped exponential backoff.
	// Does nothing if the connection has been disposed.
	const scheduleReconnect = () => {
		if (disposed) {
			return;
		}
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
		}
		const delay = Math.min(baseMs * factor ** reconnectAttempt, maxMs);
		reconnectAttempt += 1;
		reconnectTimer = setTimeout(connect, delay);
	};

	function connect() {
		if (disposed) {
			return;
		}
		if (activeSocket) {
			activeSocket.close();
		}

		const socket = connectFn();
		activeSocket = socket;

		const handleOpen = () => {
			// Connection succeeded — reset backoff.
			reconnectAttempt = 0;
			onOpen?.(socket);
		};

		const handleDisconnect = () => {
			// Guard against duplicate calls: browsers fire both
			// "error" and "close" on a failed WebSocket, so we
			// only process the first event per socket instance.
			if (activeSocket !== socket || disposed) {
				return;
			}
			activeSocket = null;
			onDisconnect?.(reconnectAttempt);
			scheduleReconnect();
		};

		socket.addEventListener("open", handleOpen);
		socket.addEventListener("error", handleDisconnect);
		socket.addEventListener("close", handleDisconnect);
	}

	// Kick off the first connection.
	connect();

	// Return a dispose function that tears everything down.
	return () => {
		disposed = true;
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
		}
		if (activeSocket) {
			activeSocket.close();
		}
	};
}
