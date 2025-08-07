import type { WebSocketEventType } from "utils/OneWayWebSocket";

type SocketSendData = string | ArrayBufferLike | Blob | ArrayBufferView;

export type MockWebSocketServer = Readonly<{
	publishMessage: (event: MessageEvent<string>) => void;
	publishError: (event: Event) => void;
	publishClose: (event: CloseEvent) => void;
	publishOpen: (event: Event) => void;

	readonly isConnectionOpen: boolean;
	readonly clientSentData: readonly SocketSendData[];
}>;

type EventMap = {
	message: MessageEvent<string>;
	error: Event;
	close: CloseEvent;
	open: Event;
};

type CallbackStore = {
	[K in keyof EventMap]: Set<(event: EventMap[K]) => void>;
};

export function createMockWebSocket(
	url: string,
	protocol?: string,
): readonly [WebSocket, MockWebSocketServer] {
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

	const mockSocket: WebSocket = {
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

		send: (data) => {
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
