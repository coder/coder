import type { WebSocketEventType } from "utils/OneWayWebSocket";

export type MockWebSocketPublisher = Readonly<{
	publishMessage: (event: MessageEvent<string>) => void;
	publishError: (event: Event) => void;
	publishClose: (event: CloseEvent) => void;
	publishOpen: (event: Event) => void;
	isConnectionOpen: () => boolean;
}>;

export function createMockWebSocket(
	url: string,
	protocols?: string | string[],
): readonly [WebSocket, MockWebSocketPublisher] {
	type EventMap = {
		message: MessageEvent<string>;
		error: Event;
		close: CloseEvent;
		open: Event;
	};
	type CallbackStore = {
		[K in keyof EventMap]: ((event: EventMap[K]) => void)[];
	};

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
		send: jest.fn(),
		dispatchEvent: jest.fn(),

		addEventListener: <E extends WebSocketEventType>(
			eventType: E,
			callback: WebSocketEventMap[E],
		) => {
			if (!isOpen) {
				return;
			}

			const subscribers = store[eventType];
			const cb = callback as unknown as CallbackStore[E][0];
			if (!subscribers.includes(cb)) {
				subscribers.push(cb);
			}
		},

		removeEventListener: <E extends WebSocketEventType>(
			eventType: E,
			callback: WebSocketEventMap[E],
		) => {
			if (!isOpen) {
				return;
			}

			const subscribers = store[eventType];
			const cb = callback as unknown as CallbackStore[E][0];
			if (subscribers.includes(cb)) {
				const updated = store[eventType].filter((c) => c !== cb);
				store[eventType] = updated as unknown as CallbackStore[E];
			}
		},

		close: () => {
			isOpen = false;
		},
	};

	const publisher: MockWebSocketPublisher = {
		isConnectionOpen: () => isOpen,
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
