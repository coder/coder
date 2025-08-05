import type { WebSocketEventType } from "utils/OneWayWebSocket";

type SocketSendData = string | ArrayBufferLike | Blob | ArrayBufferView;

export type MockWebSocketServer = Readonly<{
	publishMessage: (event: MessageEvent<string>) => void;
	publishError: (event: Event) => void;
	publishClose: (event: CloseEvent) => void;
	publishOpen: (event: Event) => void;

	readonly isConnectionOpen: boolean;
	readonly socketSendArguments: readonly SocketSendData[];
}>;

export function createMockWebSocket(
	url: string,
	protocols?: string | string[],
): readonly [WebSocket, MockWebSocketServer] {
	type EventMap = {
		message: MessageEvent<string>;
		error: Event;
		close: CloseEvent;
		open: Event;
	};
	type CallbackStore = {
		[K in keyof EventMap]: ((event: EventMap[K]) => void)[];
	};

	if (!url.startsWith("ws://") && !url.startsWith("wss://")) {
		throw new Error("URL must start with ws:// or wss://");
	}

	let activeProtocol: string;
	if (Array.isArray(protocols)) {
		activeProtocol = protocols[0] ?? "";
	} else if (typeof protocols === "string") {
		activeProtocol = protocols;
	} else {
		activeProtocol = "";
	}

	let isOpen = true;
	const store: CallbackStore = {
		message: [],
		error: [],
		close: [],
		open: [],
	};

	const sendData: SocketSendData[] = [];

	const mockSocket: WebSocket = {
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

		send: (data) => {
			if (!isOpen) {
				return;
			}
			sendData.push(data);
		},

		addEventListener: <E extends WebSocketEventType>(
			eventType: E,
			callback: (event: WebSocketEventMap[E]) => void,
		) => {
			if (!isOpen) {
				return;
			}

			const subscribers = store[eventType];
			if (!subscribers.includes(callback)) {
				subscribers.push(callback);
			}
		},

		removeEventListener: <E extends WebSocketEventType>(
			eventType: E,
			callback: (event: WebSocketEventMap[E]) => void,
		) => {
			if (!isOpen) {
				return;
			}

			const subscribers = store[eventType];
			if (subscribers.includes(callback)) {
				const updated = store[eventType].filter((c) => c !== callback);
				store[eventType] = updated as CallbackStore[E];
			}
		},

		close: () => {
			isOpen = false;
		},
	};

	const publisher: MockWebSocketServer = {
		get isConnectionOpen() {
			return isOpen;
		},

		get socketSendArguments() {
			return [...sendData];
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
