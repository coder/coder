// This file is temporary until we have a proper websocket implementation for dynamic parameters
import { useCallback, useEffect, useRef, useState } from "react";

export function useWebSocket<T>(
	url: string,
	testdata: string,
	user: string,
	plan: string,
) {
	const [message, setMessage] = useState<T | null>(null);
	const [connectionStatus, setConnectionStatus] = useState<
		"connecting" | "connected" | "disconnected"
	>("connecting");
	const wsRef = useRef<WebSocket | null>(null);
	const urlRef = useRef(url);

	const connectWebSocket = useCallback(() => {
		try {
			const ws = new WebSocket(urlRef.current);
			wsRef.current = ws;
			setConnectionStatus("connecting");

			ws.onopen = () => {
				// console.log("Connected to WebSocket");
				setConnectionStatus("connected");
				ws.send(JSON.stringify({}));
			};

			ws.onmessage = (event) => {
				try {
					const data: T = JSON.parse(event.data);
					// console.log("Received message:", data);
					setMessage(data);
				} catch (err) {
					console.error("Invalid JSON from server: ", event.data);
					console.error("Error: ", err);
				}
			};

			ws.onerror = (event) => {
				console.error("WebSocket error:", event);
			};

			ws.onclose = (event) => {
				// console.log(
				// 	`WebSocket closed with code ${event.code}. Reason: ${event.reason}`,
				// );
				setConnectionStatus("disconnected");
			};
		} catch (error) {
			console.error("Failed to create WebSocket connection:", error);
			setConnectionStatus("disconnected");
		}
	}, []);

	useEffect(() => {
		if (!testdata) {
			return;
		}

		setMessage(null);
		setConnectionStatus("connecting");

		const createConnection = () => {
			urlRef.current = url;
			connectWebSocket();
		};

		if (wsRef.current) {
			wsRef.current.close();
			wsRef.current = null;
		}

		const timeoutId = setTimeout(createConnection, 100);

		return () => {
			clearTimeout(timeoutId);
			if (wsRef.current) {
				wsRef.current.close();
				wsRef.current = null;
			}
		};
	}, [testdata, connectWebSocket, url]);

	const sendMessage = (data: unknown) => {
		if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
			wsRef.current.send(JSON.stringify(data));
		} else {
			console.warn("Cannot send message: WebSocket is not connected");
		}
	};

	return { message, sendMessage, connectionStatus };
}
