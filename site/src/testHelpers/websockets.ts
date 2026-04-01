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

type MockWebSocket = Omit<WebSocket, "send"> & {
	/**
	 * A version of the WebSocket `send` method that has been pre-wrapped inside
	 * a Jest mock.
	 *
	 * The Jest mock functionality should be used at a minimum. Basically:
	 * 1. If you want to check that the mock socket sent something to the mock
	 *    server: call the `send` method as a function, and then check the
	 *    `clientSentData` on `MockWebSocketServer` to see what data got
	 *    received.
	 * 2. If you need to make sure that the client-side `send` method got called
	 *    at all: you can use the Jest mock functionality, but you should
	 *    probably also be checking `clientSentData` still and making additional
	 *    assertions with it.
	 *
	 * Generally, tests should center around whether socket-to-server
	 * communication was successful, not whether the client-side method was
	 * called.
	 */
	send: jest.Mock<void, [SocketSendData], unknown>;
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
