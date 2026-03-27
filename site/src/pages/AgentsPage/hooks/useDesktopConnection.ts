import RFB from "@novnc/novnc/lib/rfb";
import { useClipboard } from "hooks/useClipboard";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { watchChatDesktop } from "#/api/api";

interface UseDesktopConnectionOptions {
	chatId: string | undefined;
}

type DesktopConnectionStatus =
	| "idle"
	| "connecting"
	| "connected"
	| "disconnected"
	| "error";

export interface UseDesktopConnectionResult {
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
	/** Latest text received from the remote desktop clipboard. */
	remoteClipboardText: string | null;
	/** The underlying RFB instance, if connected. */
	rfb: RFB | null;
}

const MAX_BACKOFF_MS = 30_000;
const MAX_RECONNECT_ATTEMPTS = 10;
const STABLE_CONNECTION_MS = 3_000;
const CONNECT_TIMEOUT_MS = 30_000;

// X11 keysym values sent to the remote desktop via the RFB
// protocol. Full list: https://www.cl.cam.ac.uk/~mgk25/ucs/keysymdef.h
const XK_Control_L = 0xffe3; // Left Control modifier.
const XK_Control_R = 0xffe4; // Right Control modifier.
const XK_Meta_L = 0xffeb; // Left Meta (Cmd on macOS) modifier.
const XK_Meta_R = 0xffec; // Right Meta modifier.
const XK_c = 0x0063; // Latin lowercase 'c'.
const XK_v = 0x0076; // Latin lowercase 'v'.
const XK_x = 0x0078; // Latin lowercase 'x'.
const XK_Shift_L = 0xffe1; // Left Shift modifier.
const XK_Shift_R = 0xffe2; // Right Shift modifier.

const isPasteShortcut = (event: KeyboardEvent): boolean => {
	const key = event.key.toLowerCase();
	return (
		(key === "v" && (event.ctrlKey || event.metaKey) && !event.altKey) ||
		(key === "insert" && event.shiftKey && !event.ctrlKey && !event.metaKey)
	);
};

// Detect Cmd+C on macOS so we can remap it to Ctrl+C on the
// remote desktop. Plain Ctrl+C already travels through noVNC
// correctly, so we only need to intercept the Meta variant.
const isMacCopyShortcut = (event: KeyboardEvent): boolean => {
	const key = event.key.toLowerCase();
	return key === "c" && event.metaKey && !event.ctrlKey && !event.altKey;
};

// Detect Cmd+X on macOS — same remapping rationale as copy.
const isMacCutShortcut = (event: KeyboardEvent): boolean => {
	const key = event.key.toLowerCase();
	return key === "x" && event.metaKey && !event.ctrlKey && !event.altKey;
};

export function useDesktopConnection({
	chatId,
}: UseDesktopConnectionOptions): UseDesktopConnectionResult {
	const [status, setStatus] = useState<DesktopConnectionStatus>("idle");
	const [hasConnected, setHasConnected] = useState(false);
	const [remoteClipboardText, setRemoteClipboardText] = useState<string | null>(
		null,
	);

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
	const { copyToClipboard: syncRemoteClipboardToLocal } = useClipboard({
		onError: () => {
			toast.error(
				"Failed to sync the remote clipboard to your local clipboard.",
			);
		},
	});
	// Stable ref so the effect can call the latest
	// syncRemoteClipboardToLocal without listing it as a
	// dependency (its identity may change across renders).
	const syncClipboardRef = useRef(syncRemoteClipboardToLocal);
	useEffect(() => {
		syncClipboardRef.current = syncRemoteClipboardToLocal;
	}, [syncRemoteClipboardToLocal]);

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
		let visibilityObserver: ResizeObserver | null = null;

		setRemoteClipboardText(null);

		let clipboardKeyListener: ((event: KeyboardEvent) => void) | null = null;
		let clipboardKeyUpListener: ((event: KeyboardEvent) => void) | null = null;
		const removeClipboardKeyListener = () => {
			const screen = offscreenContainerRef.current;
			if (screen && clipboardKeyListener) {
				screen.removeEventListener("keydown", clipboardKeyListener, true);
				clipboardKeyListener = null;
			}
			if (screen && clipboardKeyUpListener) {
				screen.removeEventListener("keyup", clipboardKeyUpListener, true);
				clipboardKeyUpListener = null;
			}
		};

		const cleanupRfb = () => {
			removeClipboardKeyListener();
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
			visibilityObserver?.disconnect();
			visibilityObserver = null;
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
			visibilityObserver?.disconnect();
			visibilityObserver = null;
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
			offscreenContainerRef.current.style.position = "relative";

			const socket = watchChatDesktop(chatId);

			try {
				const rfb = new RFB(offscreenContainerRef.current, socket, {
					shared: true,
				});

				rfb.scaleViewport = true;
				rfb.resizeSession = false;
				rfb.focusOnClick = true;

				// Per-session flags scoped to this RFB instance.
				// NOT refs — each doConnect() gets fresh copies so
				// state from a previous session cannot leak.
				let sessionConnected = false;
				let securityFailed = false;

				rfb.addEventListener("clipboard", (event) => {
					const text = event.detail.text ?? "";
					if (gen !== generationRef.current || !sessionConnected) {
						return;
					}
					setRemoteClipboardText(text);
					syncClipboardRef.current(text).catch((err) => {
						console.error("Failed to sync remote clipboard to local:", err);
					});
				});

				// Capture-phase keydown on the container fires before
				// noVNC's own handlers on the canvas. This lets us
				// intercept clipboard shortcuts before noVNC swallows
				// them without any hidden-textarea focus tricks.
				// Track which keys were intercepted on keydown so
				// the corresponding keyup events can be suppressed.
				const interceptedKeys = new Set<string>();

				clipboardKeyListener = (event) => {
					if (gen !== generationRef.current) {
						return;
					}
					if (!sessionConnected) {
						return;
					}
					// Remap Cmd+C / Cmd+X on macOS to Ctrl+C / Ctrl+X
					// on the remote desktop. Release the stale Meta
					// modifier first because its keydown already reached
					// the remote before we could intercept it.
					if (isMacCopyShortcut(event) || isMacCutShortcut(event)) {
						const targetKeysym = isMacCopyShortcut(event) ? XK_c : XK_x;
						const keyName = isMacCopyShortcut(event) ? "KeyC" : "KeyX";
						event.preventDefault();
						event.stopPropagation();
						interceptedKeys.add(event.code);
						rfb.sendKey(XK_Meta_L, "MetaLeft", false);
						rfb.sendKey(XK_Meta_R, "MetaRight", false);
						rfb.sendKey(XK_Control_L, "ControlLeft", true);
						rfb.sendKey(targetKeysym, keyName, true);
						rfb.sendKey(targetKeysym, keyName, false);
						rfb.sendKey(XK_Control_L, "ControlLeft", false);
						return;
					}

					if (!isPasteShortcut(event)) {
						return;
					}
					event.preventDefault();
					event.stopPropagation();
					interceptedKeys.add(event.code);

					const doPaste = async () => {
						// Check clipboard permission state before
						// attempting readText(). This lets us show a
						// clear message when permission is denied
						// instead of silently hanging.
						try {
							const permStatus = await navigator.permissions.query({
								name: "clipboard-read" as PermissionName,
							});
							if (permStatus.state === "denied") {
								toast.error(
									"Clipboard permission denied. Allow clipboard access in your browser settings to paste.",
								);
								return;
							}
						} catch {
							// permissions.query may not support
							// clipboard-read in all browsers.
							// Continue with readText anyway.
						}

						const text = await navigator.clipboard.readText();
						if (!text) {
							return;
						}
						if (gen !== generationRef.current) {
							return;
						}
						rfb.clipboardPasteFrom(text);
						rfb.sendKey(XK_Shift_L, "ShiftLeft", false);
						rfb.sendKey(XK_Shift_R, "ShiftRight", false);
						rfb.sendKey(XK_Meta_L, "MetaLeft", false);
						rfb.sendKey(XK_Meta_R, "MetaRight", false);
						rfb.sendKey(XK_Control_L, "ControlLeft", false);
						rfb.sendKey(XK_Control_R, "ControlRight", false);
						rfb.sendKey(XK_Control_L, "ControlLeft", true);
						rfb.sendKey(XK_v, "KeyV", true);
						rfb.sendKey(XK_v, "KeyV", false);
						rfb.sendKey(XK_Control_L, "ControlLeft", false);
					};
					doPaste().catch((err) => {
						console.error("Paste into remote desktop failed:", err);
					});
				};

				clipboardKeyUpListener = (event) => {
					if (interceptedKeys.has(event.code)) {
						interceptedKeys.delete(event.code);
						event.preventDefault();
						event.stopPropagation();
					}
				};

				offscreenContainerRef.current.addEventListener(
					"keydown",
					clipboardKeyListener,
					true,
				);
				offscreenContainerRef.current.addEventListener(
					"keyup",
					clipboardKeyUpListener,
					true,
				);

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

				// Work around a noVNC rendering bug: when an ancestor
				// hides this container (display: none), the canvas
				// shrinks to 0×0. When the container becomes visible
				// again, noVNC may skip rescaling because it believes
				// the viewport size hasn't changed. Re-assigning
				// scaleViewport = true forces a fresh scale pass
				// regardless.
				let prevContainerW = 0;
				let prevContainerH = 0;
				visibilityObserver = new ResizeObserver((entries) => {
					const entry = entries[0];
					if (!entry) return;
					if (gen !== generationRef.current) return;
					const { width, height } = entry.contentRect;
					const wasHidden = prevContainerW === 0 && prevContainerH === 0;
					const isVisible = width > 0 && height > 0;
					prevContainerW = width;
					prevContainerH = height;
					if (wasHidden && isVisible && rfbRef.current) {
						rfbRef.current.scaleViewport = true;
					}
				});
				visibilityObserver.observe(offscreenContainerRef.current);
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
		remoteClipboardText,
		rfb: rfbInstance,
	};
}
