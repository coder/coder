import RFB from "@novnc/novnc/lib/rfb";
import { watchChatDesktop } from "#/api/api";
import { toast } from "#/components/Toaster/toast";

export type DesktopConnectionStatus =
	| "idle"
	| "connecting"
	| "connected"
	| "disconnected"
	| "error";

type DesktopConnectionSnapshot = {
	readonly status: DesktopConnectionStatus;
	/** Whether a session of the current chat has ever connected. */
	readonly hasConnected: boolean;
};

const idleSnapshot: DesktopConnectionSnapshot = {
	status: "idle",
	hasConnected: false,
};

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

// Detect Cmd+X on macOS, remapped for the same reason as copy.
const isMacCutShortcut = (event: KeyboardEvent): boolean => {
	const key = event.key.toLowerCase();
	return key === "x" && event.metaKey && !event.ctrlKey && !event.altKey;
};

/**
 * A noVNC connection to a chat's desktop. After a dropped session it
 * reconnects with exponential backoff. Settings apply to the current
 * session and every later one. Status changes are published to
 * subscribers, in the shape `useSyncExternalStore` reads.
 */
export class DesktopConnection {
	private snapshot = idleSnapshot;
	private readonly listeners = new Set<() => void>();

	private chatId: string | undefined;
	private onRemoteClipboard: (text: string) => Promise<unknown> = () =>
		Promise.resolve();
	private scaleViewport = false;

	private rfb: RFB | null = null;
	// Offscreen container for the RFB canvas; moved into the visible
	// panel by `attach()`.
	private screen: HTMLElement | null = null;
	private visibilityObserver: ResizeObserver | null = null;
	private removeClipboardKeyListeners: (() => void) | null = null;

	private reconnectAttempt = 0;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private reconnectStableTimer: ReturnType<typeof setTimeout> | null = null;
	private connectTimeout: ReturnType<typeof setTimeout> | null = null;
	// Incremented at the start of every session and on teardown. Event
	// handlers capture the current value and bail when it no longer
	// matches, which prevents async noVNC callbacks from a previous
	// session (e.g. a phantom "disconnect" event fired after cleanupRfb)
	// from scheduling unwanted reconnect timers.
	private generation = 0;

	subscribe = (listener: () => void): (() => void) => {
		this.listeners.add(listener);
		return () => {
			this.listeners.delete(listener);
		};
	};

	getSnapshot = (): DesktopConnectionSnapshot => this.snapshot;

	/** Connects to the chat's desktop, replacing any current session. */
	open = (
		chatId: string,
		onRemoteClipboard: (text: string) => Promise<unknown>,
	) => {
		this.teardown();
		this.chatId = chatId;
		this.onRemoteClipboard = onRemoteClipboard;
		this.connect();
	};

	/** Ends the session and stops reconnecting. */
	close = () => {
		this.teardown();
		this.chatId = undefined;
		this.setSnapshot(idleSnapshot);
	};

	/** Starts a fresh session with a reset retry budget. */
	reconnect = () => {
		if (!this.chatId) {
			return;
		}
		this.teardown();
		this.connect();
	};

	/** Scales the remote screen to fit its container, or shows it at 100%. */
	setScaleViewport = (scaleViewport: boolean) => {
		this.scaleViewport = scaleViewport;
		if (this.rfb) {
			this.rfb.scaleViewport = scaleViewport;
		}
	};

	/**
	 * Moves the noVNC canvas into a container element without
	 * reconnecting. Can be called again, e.g. when the tab is reselected.
	 */
	attach = (container: HTMLElement) => {
		if (this.screen && this.screen.parentElement !== container) {
			container.appendChild(this.screen);
		}
	};

	private setSnapshot(update: Partial<DesktopConnectionSnapshot>) {
		this.snapshot = { ...this.snapshot, ...update };
		for (const listener of this.listeners) {
			listener();
		}
	}

	private cleanupRfb() {
		this.removeClipboardKeyListeners?.();
		this.removeClipboardKeyListeners = null;
		if (this.rfb) {
			try {
				this.rfb.disconnect();
			} catch {
				// Ignore errors during disconnect.
			}
			this.rfb = null;
		}
	}

	private clearAllTimers() {
		if (this.reconnectTimer !== null) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		if (this.reconnectStableTimer !== null) {
			clearTimeout(this.reconnectStableTimer);
			this.reconnectStableTimer = null;
		}
		if (this.connectTimeout !== null) {
			clearTimeout(this.connectTimeout);
			this.connectTimeout = null;
		}
	}

	private teardown() {
		this.generation++;
		this.clearAllTimers();
		this.cleanupRfb();
		this.screen?.remove();
		this.screen = null;
		this.reconnectAttempt = 0;
		this.visibilityObserver?.disconnect();
		this.visibilityObserver = null;
	}

	private connect = () => {
		const chatId = this.chatId;
		if (!chatId) {
			return;
		}

		// Bump the generation so any in-flight async callbacks
		// from the previous RFB session become stale.
		this.generation++;
		const gen = this.generation;
		const isStale = () => gen !== this.generation;

		this.clearAllTimers();
		this.cleanupRfb();
		this.visibilityObserver?.disconnect();
		this.visibilityObserver = null;
		this.setSnapshot({ status: "connecting" });

		// Remove the previous container from the DOM so reconnect
		// cycles don't leak detached divs.
		this.screen?.remove();
		const screen = document.createElement("div");
		screen.style.width = "100%";
		screen.style.height = "100%";
		screen.style.position = "relative";
		screen.style.overflow = "hidden";
		this.screen = screen;

		const socket = watchChatDesktop(chatId);

		try {
			const rfb = new RFB(screen, socket, {
				shared: true,
			});

			rfb.scaleViewport = this.scaleViewport;
			rfb.resizeSession = false;
			rfb.focusOnClick = true;

			// Override the noVNC default background (rgb(40,40,40))
			// so the letterbox margins match the app surface color
			// in both light and dark themes.
			const surfaceHsl = getComputedStyle(document.documentElement)
				.getPropertyValue("--surface-secondary")
				.trim();
			if (surfaceHsl) {
				rfb.background = `hsl(${surfaceHsl})`;
			}

			// Per-session flags scoped to this RFB instance, so state
			// from a previous session cannot leak.
			let sessionConnected = false;
			let securityFailed = false;

			rfb.addEventListener("clipboard", (event) => {
				const text = event.detail.text ?? "";
				if (isStale() || !sessionConnected) {
					return;
				}
				this.onRemoteClipboard(text).catch((err) => {
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

			const clipboardKeyListener = (event: KeyboardEvent) => {
				if (isStale()) {
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
					if (isStale()) {
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

			const clipboardKeyUpListener = (event: KeyboardEvent) => {
				if (interceptedKeys.has(event.code)) {
					interceptedKeys.delete(event.code);
					event.preventDefault();
					event.stopPropagation();
				}
			};

			screen.addEventListener("keydown", clipboardKeyListener, true);
			screen.addEventListener("keyup", clipboardKeyUpListener, true);
			this.removeClipboardKeyListeners = () => {
				screen.removeEventListener("keydown", clipboardKeyListener, true);
				screen.removeEventListener("keyup", clipboardKeyUpListener, true);
			};

			// Fail if the VNC handshake doesn't complete within
			// a reasonable window. Bump the generation before
			// cleanupRfb() so the resulting noVNC "disconnect"
			// event is treated as stale and doesn't redundantly
			// set status to "error".
			this.connectTimeout = setTimeout(() => {
				if (isStale()) return;
				if (!sessionConnected) {
					this.generation++;
					this.cleanupRfb();
					this.setSnapshot({ status: "error" });
				}
			}, CONNECT_TIMEOUT_MS);

			rfb.addEventListener("connect", () => {
				if (isStale()) return;
				sessionConnected = true;
				if (this.connectTimeout !== null) {
					clearTimeout(this.connectTimeout);
					this.connectTimeout = null;
				}
				this.setSnapshot({ status: "connected", hasConnected: true });
				// Only reset the reconnect counter after the
				// connection has been stable for a minimum
				// duration. This prevents infinite reconnect
				// loops when the VNC handshake succeeds but the
				// connection drops immediately.
				// No gen check needed: clearAllTimers()
				// always cancels this before a new session.
				this.reconnectStableTimer = setTimeout(() => {
					this.reconnectAttempt = 0;
				}, STABLE_CONNECTION_MS);
			});

			rfb.addEventListener("disconnect", () => {
				if (isStale()) return;
				if (this.reconnectStableTimer !== null) {
					clearTimeout(this.reconnectStableTimer);
					this.reconnectStableTimer = null;
				}
				if (this.connectTimeout !== null) {
					clearTimeout(this.connectTimeout);
					this.connectTimeout = null;
				}
				this.rfb = null;

				// Security failures are terminal: the
				// securityfailure handler already moved to
				// "error". noVNC fires disconnect after
				// securityfailure; ignore it so we don't
				// accidentally schedule a retry.
				if (securityFailed) {
					return;
				}

				// Only retry if THIS session's VNC handshake
				// completed. A previous session having connected
				// is irrelevant; the desktop may have become
				// permanently unavailable.
				if (!sessionConnected) {
					this.setSnapshot({ status: "error" });
					return;
				}

				const attempt = this.reconnectAttempt;

				if (attempt >= MAX_RECONNECT_ATTEMPTS) {
					this.setSnapshot({ status: "error" });
					return;
				}

				this.setSnapshot({ status: "disconnected" });

				const delay = Math.min(1000 * 2 ** attempt, MAX_BACKOFF_MS);
				this.reconnectAttempt = attempt + 1;
				this.reconnectTimer = setTimeout(this.connect, delay);
			});

			rfb.addEventListener("securityfailure", () => {
				if (isStale()) return;
				securityFailed = true;
				this.rfb = null;
				this.setSnapshot({ status: "error" });
			});

			this.rfb = rfb;

			// Work around a noVNC rendering bug: when an ancestor
			// hides this container (display: none), the canvas
			// shrinks to 0×0. When the container becomes visible
			// again, noVNC may skip rescaling because it believes
			// the viewport size hasn't changed. Re-assigning
			// scaleViewport forces a fresh scale pass
			// regardless.
			let prevContainerW = 0;
			let prevContainerH = 0;
			this.visibilityObserver = new ResizeObserver((entries) => {
				const entry = entries[0];
				if (!entry) return;
				if (isStale()) return;
				const { width, height } = entry.contentRect;
				const wasHidden = prevContainerW === 0 && prevContainerH === 0;
				const isVisible = width > 0 && height > 0;
				prevContainerW = width;
				prevContainerH = height;
				if (wasHidden && isVisible && this.rfb) {
					// Re-assign the current value to force noVNC to
					// recalculate the viewport. The setter triggers an
					// internal rescale regardless of whether the value
					// actually changed.
					const current = this.rfb.scaleViewport;
					this.rfb.scaleViewport = current;
				}
			});
			this.visibilityObserver.observe(screen);
		} catch {
			socket.close();
			this.setSnapshot({ status: "error" });
		}
	};
}
