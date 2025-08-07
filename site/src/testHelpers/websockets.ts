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

export type MockWebSocket = Omit<WebSocket, "send"> & {
	/**
	 * A version of the WebSocket `send` method that has been pre-wrapped inside
	 * a Jest mock. Most of the time, you should not be using the Jest mock
	 * features, and should instead be using the `clientSentData` property from
	 * the `MockWebSocketServer` type.
	 *
	 * The Jest mock functionality only exists to help make sure that the
	 * client-side socket method got called. It does nothing to make sure that
	 * the mock server actually received anything.
	 */
	send: jest.Mock<void, [SocketSendData], unknown>;
};

export function createMockWebSocket(
	url: string,
	protocol?: string,
): readonly [MockWebSocket, MockWebSocketServer] {
	if (!url.startsWith("ws://") && !url.startsWith("wss://")) {
		throw new Error("URL must start with ws:// or wss://");
	}

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
		protocol: protocol ?? "",
		readyState: 1,
		binaryType: "blob",
		bufferedAmount: 0,
		extensions: "",
		onclose: null,
		onerror: null,
		onmessage: null,
		onopen: null,
		dispatchEvent: jest.fn(),

		send: jest.fn((data) => {
			if (!isOpen) {
				return;
			}
			sentData.push(data);
		}),

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
