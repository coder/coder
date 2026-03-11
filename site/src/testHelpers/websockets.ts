import type { WebSocketEventType } from "utils/OneWayWebSocket";

type SocketSendData = Parameters<WebSocket["send"]>[0];

export type MockWebSocketServer = Readonly<{
	publishMessage: (event: MessageEvent<string>) => void;
	publishError: (event: Event) => void;
	publishClose: (event: CloseEvent) => void;
	publishOpen: (event: Event) => void;

	readonly isConnectionOpen: boolean;
	readonly clientSentData: readonly SocketSendData[];
}>;

type CallbackStore = {
	[K in keyof WebSocketEventMap]: Set<(event: WebSocketEventMap[K]) => void>;
};

type MockWebSocket = Omit<WebSocket, "send" | "dispatchEvent"> & {
	/**
	 * If you want to check that the mock socket sent something to the
	 * mock server, check `clientSentData` on `MockWebSocketServer`.
	 */
	send: (data: SocketSendData) => void;
	dispatchEvent: (event: Event) => boolean;
};

export function createMockWebSocket(
	url: string,
	protocol?: string | string[] | undefined,
): readonly [MockWebSocket, MockWebSocketServer] {
	if (!url.startsWith("ws://") && !url.startsWith("wss://")) {
		throw new Error("URL must start with ws:// or wss://");
	}

	const activeProtocol = Array.isArray(protocol)
		? protocol.join(" ")
		: (protocol ?? "");

	let isOpen = true;
	const store: CallbackStore = {
		message: new Set(),
		error: new Set(),
		close: new Set(),
		open: new Set(),
	};

	const sentData: SocketSendData[] = [];

	const mockSocket: MockWebSocket = {
		CONNECTING: 0,
		OPEN: 1,
		CLOSING: 2,
		CLOSED: 3,

		url,
		protocol: activeProtocol,
		readyState: 1,
		binaryType: "blob",
		bufferedAmount: 0,
		extensions: "",
		onclose: null,
		onerror: null,
		onmessage: null,
		onopen: null,
		dispatchEvent: () => false,

		send: (data: SocketSendData) => {
			if (!isOpen) {
				return;
			}
			sentData.push(data);
		},

		addEventListener: <E extends WebSocketEventType>(
			eventType: E,
			callback: (event: WebSocketEventMap[E]) => void,
		) => {
			if (!isOpen) {
				return;
			}
			const subscribers = store[eventType];
			subscribers.add(callback);
		},

		removeEventListener: <E extends WebSocketEventType>(
			eventType: E,
			callback: (event: WebSocketEventMap[E]) => void,
		) => {
			if (!isOpen) {
				return;
			}
			const subscribers = store[eventType];
			subscribers.delete(callback);
		},

		close: () => {
			isOpen = false;
		},
	};

	const publisher: MockWebSocketServer = {
		get isConnectionOpen() {
			return isOpen;
		},

		get clientSentData() {
			return [...sentData];
		},

		publishOpen: (event) => {
			if (!isOpen) {
				return;
			}
			for (const sub of store.open) {
				sub(event);
			}
		},

		publishError: (event) => {
			if (!isOpen) {
				return;
			}
			for (const sub of store.error) {
				sub(event);
			}
		},

		publishMessage: (event) => {
			if (!isOpen) {
				return;
			}
			for (const sub of store.message) {
				sub(event);
			}
		},

		publishClose: (event) => {
			if (!isOpen) {
				return;
			}
			for (const sub of store.close) {
				sub(event);
			}
		},
	};

	return [mockSocket, publisher] as const;
}
