import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
} from "api/typesGenerated";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useCallback, useEffect, useRef, useState } from "react";
import {
	ExponentialBackoff,
	type Websocket,
	WebsocketBuilder,
	WebsocketEvent,
} from "websocket-ts";

type WebSocketStatus =
	| "connecting"
	| "connected"
	| "disconnected"
	| "error";

interface UseDynamicParametersWebSocketOptions {
	/** Template version ID to connect to. */
	templateVersionId: string | undefined;
	/** User ID for the owner. */
	userId: string;
	/** Callback when a response is received. */
	onMessage: (response: DynamicParametersResponse) => void;
	/** Callback when the websocket reconnects. Consumer should resend form state. */
	onReconnect?: () => void;
}

interface UseDynamicParametersWebSocketResult {
	/** Send a message to the websocket. */
	sendMessage: (formValues: Record<string, string>, ownerId?: string) => void;
	/** Current connection status. */
	status: WebSocketStatus;
	/** Current error, if any. */
	error: Error | null;
	/**
	 * Trigger a manual reconnection attempt. Useful when visibility changes and
	 * the websocket may have been disconnected.
	 */
	triggerReconnect: () => void;
}

/**
 * Hook for managing a WebSocket connection to the dynamic parameters endpoint.
 *
 * Features:
 * - Automatic reconnection with exponential backoff using websocket-ts
 * - Reconnects when the page becomes visible (tab focus)
 * - Calls onReconnect so consumer can resend form state
 */
export function useDynamicParametersWebSocket({
	templateVersionId,
	userId,
	onMessage,
	onReconnect,
}: UseDynamicParametersWebSocketOptions): UseDynamicParametersWebSocketResult {
	const [status, setStatus] = useState<WebSocketStatus>("connecting");
	const [error, setError] = useState<Error | null>(null);
	// Counter to force reconnection by triggering the useEffect to re-run.
	const [reconnectCounter, setReconnectCounter] = useState(0);
	const wsRef = useRef<Websocket | null>(null);
	const wsResponseIdRef = useRef<number>(-1);

	// Stable reference to the message handler
	const handleMessage = useEffectEvent(onMessage);
	const handleReconnect = useEffectEvent(() => onReconnect?.());

	const sendMessage = useCallback(
		(formValues: Record<string, string>, ownerId?: string) => {
			const request: DynamicParametersRequest = {
				id: wsResponseIdRef.current + 1,
				owner_id: ownerId ?? userId,
				inputs: formValues,
			};
			if (wsRef.current?.underlyingWebsocket?.readyState === WebSocket.OPEN) {
				wsRef.current.send(JSON.stringify(request));
				wsResponseIdRef.current = wsResponseIdRef.current + 1;
			}
		},
		[userId],
	);

	const triggerReconnect = useCallback(() => {
		const ws = wsRef.current;
		const underlyingWs = ws?.underlyingWebsocket;
		if (
			!underlyingWs ||
			underlyingWs.readyState === WebSocket.CLOSED ||
			underlyingWs.readyState === WebSocket.CLOSING
		) {
			// Close the existing websocket and increment the counter to force
			// the useEffect to create a new connection.
			ws?.close();
			setReconnectCounter((c) => c + 1);
			setStatus("connecting");
			setError(null);
		}
	}, []);

	// biome-ignore lint/correctness/useExhaustiveDependencies: reconnectCounter is intentionally included to force reconnection
	useEffect(() => {
		if (!templateVersionId) {
			return;
		}

		const protocol = location.protocol === "https:" ? "wss:" : "ws:";
		const params = new URLSearchParams({ user_id: userId });
		const url = `${protocol}//${location.host}/api/v2/templateversions/${templateVersionId}/dynamic-parameters?${params.toString()}`;

		let disposed = false;
		setStatus("connecting");
		setError(null);

		const websocket = new WebsocketBuilder(url)
			.withBackoff(new ExponentialBackoff(1000, 6))
			.build();

		wsRef.current = websocket;

		websocket.addEventListener(WebsocketEvent.open, () => {
			if (disposed) return;
			setStatus("connected");
			setError(null);
		});

		websocket.addEventListener(WebsocketEvent.message, (_, event) => {
			if (disposed) return;
			try {
				const response = JSON.parse(
					event.data as string,
				) as DynamicParametersResponse;
				handleMessage(response);
			} catch {
				// Ignore parse errors
			}
		});

		websocket.addEventListener(WebsocketEvent.error, () => {
			if (disposed) return;
			setStatus("error");
			setError(
				new Error("WebSocket connection for dynamic parameters failed."),
			);
		});

		websocket.addEventListener(WebsocketEvent.close, () => {
			if (disposed) return;
			setStatus("disconnected");
		});

		websocket.addEventListener(WebsocketEvent.reconnect, () => {
			if (disposed) return;
			setStatus("connected");
			setError(null);
			// Notify consumer so they can resend form state.
			handleReconnect();
		});

		return () => {
			disposed = true;
			websocket.close();
			wsRef.current = null;
		};
	}, [templateVersionId, userId, handleMessage, handleReconnect, reconnectCounter]);

	// Handle visibility change - reconnect when page becomes visible.
	useEffect(() => {
		if (!templateVersionId) {
			return;
		}

		const handleVisibilityChange = () => {
			if (document.visibilityState !== "visible") {
				return;
			}
			triggerReconnect();
		};

		document.addEventListener("visibilitychange", handleVisibilityChange);

		return () => {
			document.removeEventListener("visibilitychange", handleVisibilityChange);
		};
	}, [templateVersionId, triggerReconnect]);

	return {
		sendMessage,
		status,
		error,
		triggerReconnect,
	};
}
