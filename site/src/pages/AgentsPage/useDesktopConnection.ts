import RFB from "@novnc/novnc/lib/rfb";
import { watchChatDesktop } from "api/api";
import { useCallback, useEffect, useRef, useState } from "react";

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
	 * Start the connection. No-op if already connected/connecting.
	 * Called when the user first opens the Desktop tab.
	 */
	connect: () => void;
	/**
	 * Disconnect and clean up. Called on unmount.
	 */
	disconnect: () => void;
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

export function useDesktopConnection({
	chatId,
}: UseDesktopConnectionOptions): UseDesktopConnectionResult {
	const [status, setStatus] = useState<DesktopConnectionStatus>("idle");
	const [hasConnected, setHasConnected] = useState(false);

	const rfbRef = useRef<RFB | null>(null);
	const offscreenContainerRef = useRef<HTMLElement | null>(null);
	const reconnectAttemptRef = useRef(0);
	const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const disposedRef = useRef(false);
	// Track whether connect() has been called at least once.
	const connectRequestedRef = useRef(false);
	// Ref mirror of hasConnected so disconnect handlers can read
	// the latest value without stale closures.
	const hasConnectedRef = useRef(false);

	const cleanupRfb = useCallback(() => {
		if (rfbRef.current) {
			try {
				rfbRef.current.disconnect();
			} catch {
				// Ignore errors during disconnect.
			}
			rfbRef.current = null;
		}
	}, []);

	const doConnect = useCallback(() => {
		if (!chatId || disposedRef.current) {
			return;
		}

		cleanupRfb();
		setStatus("connecting");

		// Temporary offscreen container for the RFB canvas; moved into
		// the visible panel by `attach()`.
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

			// Track whether this particular RFB instance completed the
			// VNC handshake.
			let sessionConnected = false;

			rfb.addEventListener("connect", () => {
				if (disposedRef.current) return;
				sessionConnected = true;
				setStatus("connected");
				setHasConnected(true);
				hasConnectedRef.current = true;
				reconnectAttemptRef.current = 0;
			});

			rfb.addEventListener("disconnect", () => {
				if (disposedRef.current) return;
				rfbRef.current = null;

				if (!sessionConnected && !hasConnectedRef.current) {
					// The VNC handshake never completed and the desktop
					// has never been reachable. The endpoint is not
					// available (e.g. portabledesktop not installed,
					// no workspace, agent down). Don't retry.
					setStatus("error");
					return;
				}

				const attempt = reconnectAttemptRef.current;

				if (attempt >= MAX_RECONNECT_ATTEMPTS) {
					// Too many consecutive failures. Give up.
					setStatus("error");
					return;
				}

				setStatus("disconnected");

				// Either this session was connected and dropped, or a
				// previous session was connected (transient reconnect
				// failure). Retry with exponential backoff.
				const delay = Math.min(1000 * 2 ** attempt, MAX_BACKOFF_MS);
				reconnectAttemptRef.current = attempt + 1;
				reconnectTimerRef.current = setTimeout(doConnect, delay);
			});

			rfb.addEventListener("securityfailure", () => {
				if (disposedRef.current) return;
				rfbRef.current = null;
				setStatus("error");
			});

			rfbRef.current = rfb;
		} catch {
			setStatus("error");
		}
	}, [chatId, cleanupRfb]);

	const connect = useCallback(() => {
		if (connectRequestedRef.current) {
			return;
		}
		connectRequestedRef.current = true;
		doConnect();
	}, [doConnect]);

	const disconnect = useCallback(() => {
		if (reconnectTimerRef.current !== null) {
			clearTimeout(reconnectTimerRef.current);
			reconnectTimerRef.current = null;
		}
		cleanupRfb();
		offscreenContainerRef.current = null;
		setStatus("idle");
		connectRequestedRef.current = false;
		reconnectAttemptRef.current = 0;
	}, [cleanupRfb]);

	const attach = useCallback((container: HTMLElement) => {
		const screen = offscreenContainerRef.current;
		if (screen && screen.parentElement !== container) {
			container.appendChild(screen);
		}
	}, []);

	// Cleanup on unmount or chatId change.
	// biome-ignore lint/correctness/useExhaustiveDependencies: chatId is an intentional trigger to reset state for a new conversation
	useEffect(() => {
		disposedRef.current = false;

		return () => {
			disposedRef.current = true;
			if (reconnectTimerRef.current !== null) {
				clearTimeout(reconnectTimerRef.current);
				reconnectTimerRef.current = null;
			}
			cleanupRfb();
			offscreenContainerRef.current = null;
			setStatus("idle");
			setHasConnected(false);
			hasConnectedRef.current = false;
			connectRequestedRef.current = false;
			reconnectAttemptRef.current = 0;
		};
	}, [chatId, cleanupRfb]);

	return {
		status,
		hasConnected,
		connect,
		disconnect,
		attach,
		rfb: rfbRef.current,
	};
}
