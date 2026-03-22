import RFB from "@novnc/novnc/lib/rfb";
import { watchChatDesktop } from "api/api";
import { useEffect, useRef, useState } from "react";

interface UseDesktopConnectionOptions {
	chatId: string | undefined;
}

type DesktopConnectionStatus =
	| "idle"
	| "connecting"
	| "connected"
	| "disconnected"
	| "error";

interface UseDesktopConnectionResult {
	/** Current connection status. */
	status: DesktopConnectionStatus;
	/** Whether the connection has ever been established. */
	hasConnected: boolean;
	/**
	 * Tear down the current connection and start a fresh one.
	 * Used by the "Reconnect" button after an error.
	 */
	reconnect: () => void;
	/**
	 * Attach the noVNC canvas to a container element. Can be called
	 * multiple times (e.g., when the tab is re-selected). The RFB
	 * instance moves its existing canvas into the new container
	 * without reconnecting.
	 */
	attach: (container: HTMLElement) => void;
	/** The underlying RFB instance, if connected. */
	rfb: RFB | null;
}

const MAX_BACKOFF_MS = 30_000;
const MAX_RECONNECT_ATTEMPTS = 10;
const STABLE_CONNECTION_MS = 3_000;
const CONNECT_TIMEOUT_MS = 30_000;

export function useDesktopConnection({
	chatId,
}: UseDesktopConnectionOptions): UseDesktopConnectionResult {
	const [status, setStatus] = useState<DesktopConnectionStatus>("idle");
	const [hasConnected, setHasConnected] = useState(false);

	// rfbRef provides synchronous access for cleanup and event
	// handlers. rfbInstance (state) provides reactivity so consumers
	// re-render when the RFB instance changes.
	const [rfbInstance, setRfbInstance] = useState<RFB | null>(null);
	const rfbRef = useRef<RFB | null>(null);

	const offscreenContainerRef = useRef<HTMLElement | null>(null);
	const reconnectAttemptRef = useRef(0);
	const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const reconnectStableTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
		null,
	);
	const connectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	// Monotonically increasing counter. Incremented at the start of
	// every doConnect() and inside the cleanup. Event handlers
	// capture the current value and bail when it no longer matches,
	// which prevents async noVNC callbacks from a previous session
	// (e.g. a phantom "disconnect" event fired after cleanupRfb)
	// from scheduling unwanted reconnect timers.
	const generationRef = useRef(0);

	// Populated by the lifecycle effect so reconnect() can tear
	// down and restart the connection from outside the effect.
	// Cleared on cleanup so calls after unmount are no-ops.
	const restartRef = useRef<(() => void) | null>(null);

	const reconnect = () => {
		restartRef.current?.();
	};

	const attach = (container: HTMLElement) => {
		const screen = offscreenContainerRef.current;
		if (screen && screen.parentElement !== container) {
			container.appendChild(screen);
		}
	};

	// Single lifecycle effect that owns the entire connection.
	// Connects on mount, tears down on unmount, and resets when
	// chatId changes.
	//
	// All connection logic (doConnect, cleanupRfb, timer helpers)
	// is defined inside the effect so the dependency array only
	// contains primitives. This avoids any reliance on the React
	// Compiler for function identity stability.
	useEffect(() => {
		const cleanupRfb = () => {
			if (rfbRef.current) {
				try {
					rfbRef.current.disconnect();
				} catch {
					// Ignore errors during disconnect.
				}
				rfbRef.current = null;
				setRfbInstance(null);
			}
		};

		const clearAllTimers = () => {
			if (reconnectTimerRef.current !== null) {
				clearTimeout(reconnectTimerRef.current);
				reconnectTimerRef.current = null;
			}
			if (reconnectStableTimerRef.current !== null) {
				clearTimeout(reconnectStableTimerRef.current);
				reconnectStableTimerRef.current = null;
			}
			if (connectTimeoutRef.current !== null) {
				clearTimeout(connectTimeoutRef.current);
				connectTimeoutRef.current = null;
			}
		};

		const teardown = () => {
			generationRef.current++;
			clearAllTimers();
			cleanupRfb();
			if (offscreenContainerRef.current?.parentElement) {
				offscreenContainerRef.current.remove();
			}
			offscreenContainerRef.current = null;
			reconnectAttemptRef.current = 0;
		};

		const doConnect = () => {
			if (!chatId) {
				return;
			}

			// Bump the generation so any in-flight async callbacks
			// from the previous RFB session become stale.
			generationRef.current++;
			const gen = generationRef.current;

			clearAllTimers();
			cleanupRfb();
			setStatus("connecting");

			// Remove the previous offscreen container from the DOM
			// so reconnect cycles don't leak detached divs.
			if (offscreenContainerRef.current?.parentElement) {
				offscreenContainerRef.current.remove();
			}

			// Temporary offscreen container for the RFB canvas;
			// moved into the visible panel by `attach()`.
			offscreenContainerRef.current = document.createElement("div");
			offscreenContainerRef.current.style.width = "100%";
			offscreenContainerRef.current.style.height = "100%";

			const socket = watchChatDesktop(chatId);

			try {
				const rfb = new RFB(offscreenContainerRef.current, socket, {
					shared: true,
				});

				rfb.scaleViewport = true;
				rfb.resizeSession = false;

				// Per-session flags scoped to this RFB instance.
				// NOT refs — each doConnect() gets fresh copies so
				// state from a previous session cannot leak.
				let sessionConnected = false;
				let securityFailed = false;

				// Fail if the VNC handshake doesn't complete within
				// a reasonable window. Bump the generation before
				// cleanupRfb() so the resulting noVNC "disconnect"
				// event is treated as stale and doesn't redundantly
				// set status to "error".
				connectTimeoutRef.current = setTimeout(() => {
					if (gen !== generationRef.current) return;
					if (!sessionConnected) {
						generationRef.current++;
						cleanupRfb();
						setStatus("error");
					}
				}, CONNECT_TIMEOUT_MS);

				rfb.addEventListener("connect", () => {
					if (gen !== generationRef.current) return;
					sessionConnected = true;
					if (connectTimeoutRef.current !== null) {
						clearTimeout(connectTimeoutRef.current);
						connectTimeoutRef.current = null;
					}
					setStatus("connected");
					setHasConnected(true);
					// Only reset the reconnect counter after the
					// connection has been stable for a minimum
					// duration. This prevents infinite reconnect
					// loops when the VNC handshake succeeds but the
					// connection drops immediately.
					// No gen check needed — clearAllTimers()
					// always cancels this before a new session.
					reconnectStableTimerRef.current = setTimeout(() => {
						reconnectAttemptRef.current = 0;
					}, STABLE_CONNECTION_MS);
				});

				rfb.addEventListener("disconnect", () => {
					if (gen !== generationRef.current) return;
					if (reconnectStableTimerRef.current !== null) {
						clearTimeout(reconnectStableTimerRef.current);
						reconnectStableTimerRef.current = null;
					}
					if (connectTimeoutRef.current !== null) {
						clearTimeout(connectTimeoutRef.current);
						connectTimeoutRef.current = null;
					}
					rfbRef.current = null;
					setRfbInstance(null);

					// Security failures are terminal — the
					// securityfailure handler already moved to
					// "error". noVNC fires disconnect after
					// securityfailure; ignore it so we don't
					// accidentally schedule a retry.
					if (securityFailed) {
						return;
					}

					// Only retry if THIS session's VNC handshake
					// completed. A previous session having connected
					// is irrelevant — the desktop may have become
					// permanently unavailable.
					if (!sessionConnected) {
						setStatus("error");
						return;
					}

					const attempt = reconnectAttemptRef.current;

					if (attempt >= MAX_RECONNECT_ATTEMPTS) {
						setStatus("error");
						return;
					}

					setStatus("disconnected");

					const delay = Math.min(1000 * 2 ** attempt, MAX_BACKOFF_MS);
					reconnectAttemptRef.current = attempt + 1;
					reconnectTimerRef.current = setTimeout(doConnect, delay);
				});

				rfb.addEventListener("securityfailure", () => {
					if (gen !== generationRef.current) return;
					securityFailed = true;
					rfbRef.current = null;
					setRfbInstance(null);
					setStatus("error");
				});

				rfbRef.current = rfb;
				setRfbInstance(rfb);
			} catch {
				socket.close();
				setStatus("error");
			}
		};

		// Expose a restart handle for the Reconnect button.
		restartRef.current = () => {
			teardown();
			doConnect();
		};

		doConnect();

		return () => {
			restartRef.current = null;
			teardown();
			setStatus("idle");
			setHasConnected(false);
		};
	}, [chatId]);

	return {
		status,
		hasConnected,
		reconnect,
		attach,
		rfb: rfbInstance,
	};
}
