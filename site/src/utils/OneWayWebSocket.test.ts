/**
 * @file Sets up unit tests for OneWayWebSocket.
 *
 * 2025-03-18 - Really wanted to define these as integration tests with MSW, but
 * getting it set up correctly for Jest and JSDOM got a little screwy. That can
 * be revisited in the future, but in the meantime, we're assuming that the base
 * WebSocket class doesn't have any bugs, and can safely be mocked out.
 */

import {
	type OneWayMessageEvent,
	OneWayWebSocket,
	type WebSocketEventType,
} from "./OneWayWebSocket";

type MockPublisher = Readonly<{
	publishMessage: (event: MessageEvent<string>) => void;
	publishError: (event: ErrorEvent) => void;
	publishClose: (event: CloseEvent) => void;
	publishOpen: (event: Event) => void;
}>;

function createMockWebSocket(
	url: string,
	protocols?: string | string[],
): readonly [WebSocket, MockPublisher] {
	type EventMap = {
		message: MessageEvent<string>;
		error: ErrorEvent;
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

	let closed = false;
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
			if (closed) {
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
			if (closed) {
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
			closed = true;
		},
	};

	const publisher: MockPublisher = {
		publishOpen: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.open) {
				sub(event);
			}
		},

		publishError: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.error) {
				sub(event);
			}
		},

		publishMessage: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.message) {
				sub(event);
			}
		},

		publishClose: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.close) {
				sub(event);
			}
		},
	};

	return [mockSocket, publisher] as const;
}

describe(OneWayWebSocket.name, () => {
	const dummyRoute = "/api/v2/blah";

	it("Errors out if API route does not start with '/api/v2/'", () => {
		const testRoutes: string[] = ["blah", "", "/", "/api", "/api/v225"];

		for (const r of testRoutes) {
			expect(() => {
				new OneWayWebSocket({
					apiRoute: r,
					websocketInit: (url, protocols) => {
						const [socket] = createMockWebSocket(url, protocols);
						return socket;
					},
				});
			}).toThrow(Error);
		}
	});

	it("Lets a consumer add an event listener of each type", () => {
		let publisher!: MockPublisher;
		const oneWay = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket, pub] = createMockWebSocket(url, protocols);
				publisher = pub;
				return socket;
			},
		});

		const onOpen = jest.fn();
		const onClose = jest.fn();
		const onError = jest.fn();
		const onMessage = jest.fn();

		oneWay.addEventListener("open", onOpen);
		oneWay.addEventListener("close", onClose);
		oneWay.addEventListener("error", onError);
		oneWay.addEventListener("message", onMessage);

		publisher.publishOpen(new Event("open"));
		publisher.publishClose(new CloseEvent("close"));
		publisher.publishError(
			new ErrorEvent("error", {
				error: new Error("Whoops - connection broke"),
			}),
		);
		publisher.publishMessage(
			new MessageEvent("message", {
				data: "null",
			}),
		);

		expect(onOpen).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledTimes(1);
		expect(onError).toHaveBeenCalledTimes(1);
		expect(onMessage).toHaveBeenCalledTimes(1);
	});

	it("Lets a consumer remove an event listener of each type", () => {
		let publisher!: MockPublisher;
		const oneWay = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket, pub] = createMockWebSocket(url, protocols);
				publisher = pub;
				return socket;
			},
		});

		const onOpen = jest.fn();
		const onClose = jest.fn();
		const onError = jest.fn();
		const onMessage = jest.fn();

		oneWay.addEventListener("open", onOpen);
		oneWay.addEventListener("close", onClose);
		oneWay.addEventListener("error", onError);
		oneWay.addEventListener("message", onMessage);

		oneWay.removeEventListener("open", onOpen);
		oneWay.removeEventListener("close", onClose);
		oneWay.removeEventListener("error", onError);
		oneWay.removeEventListener("message", onMessage);

		publisher.publishOpen(new Event("open"));
		publisher.publishClose(new CloseEvent("close"));
		publisher.publishError(
			new ErrorEvent("error", {
				error: new Error("Whoops - connection broke"),
			}),
		);
		publisher.publishMessage(
			new MessageEvent("message", {
				data: "null",
			}),
		);

		expect(onOpen).toHaveBeenCalledTimes(0);
		expect(onClose).toHaveBeenCalledTimes(0);
		expect(onError).toHaveBeenCalledTimes(0);
		expect(onMessage).toHaveBeenCalledTimes(0);
	});

	it("Only calls each callback once if callback is added multiple times", () => {
		let publisher!: MockPublisher;
		const oneWay = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket, pub] = createMockWebSocket(url, protocols);
				publisher = pub;
				return socket;
			},
		});

		const onOpen = jest.fn();
		const onClose = jest.fn();
		const onError = jest.fn();
		const onMessage = jest.fn();

		for (let i = 0; i < 10; i++) {
			oneWay.addEventListener("open", onOpen);
			oneWay.addEventListener("close", onClose);
			oneWay.addEventListener("error", onError);
			oneWay.addEventListener("message", onMessage);
		}

		publisher.publishOpen(new Event("open"));
		publisher.publishClose(new CloseEvent("close"));
		publisher.publishError(
			new ErrorEvent("error", {
				error: new Error("Whoops - connection broke"),
			}),
		);
		publisher.publishMessage(
			new MessageEvent("message", {
				data: "null",
			}),
		);

		expect(onOpen).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledTimes(1);
		expect(onError).toHaveBeenCalledTimes(1);
		expect(onMessage).toHaveBeenCalledTimes(1);
	});

	it("Lets consumers register multiple callbacks for each event type", () => {
		let publisher!: MockPublisher;
		const oneWay = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket, pub] = createMockWebSocket(url, protocols);
				publisher = pub;
				return socket;
			},
		});

		const onOpen1 = jest.fn();
		const onClose1 = jest.fn();
		const onError1 = jest.fn();
		const onMessage1 = jest.fn();
		oneWay.addEventListener("open", onOpen1);
		oneWay.addEventListener("close", onClose1);
		oneWay.addEventListener("error", onError1);
		oneWay.addEventListener("message", onMessage1);

		const onOpen2 = jest.fn();
		const onClose2 = jest.fn();
		const onError2 = jest.fn();
		const onMessage2 = jest.fn();
		oneWay.addEventListener("open", onOpen2);
		oneWay.addEventListener("close", onClose2);
		oneWay.addEventListener("error", onError2);
		oneWay.addEventListener("message", onMessage2);

		publisher.publishOpen(new Event("open"));
		publisher.publishClose(new CloseEvent("close"));
		publisher.publishError(
			new ErrorEvent("error", {
				error: new Error("Whoops - connection broke"),
			}),
		);
		publisher.publishMessage(
			new MessageEvent("message", {
				data: "null",
			}),
		);

		expect(onOpen1).toHaveBeenCalledTimes(1);
		expect(onClose1).toHaveBeenCalledTimes(1);
		expect(onError1).toHaveBeenCalledTimes(1);
		expect(onMessage1).toHaveBeenCalledTimes(1);

		expect(onOpen2).toHaveBeenCalledTimes(1);
		expect(onClose2).toHaveBeenCalledTimes(1);
		expect(onError2).toHaveBeenCalledTimes(1);
		expect(onMessage2).toHaveBeenCalledTimes(1);
	});

	it("Computes the socket protocol based on the browser location protocol", () => {
		const oneWay1 = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket] = createMockWebSocket(url, protocols);
				return socket;
			},
			location: {
				protocol: "https:",
				host: "www.cool.com",
			},
		});
		const oneWay2 = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket] = createMockWebSocket(url, protocols);
				return socket;
			},
			location: {
				protocol: "http:",
				host: "www.cool.com",
			},
		});

		expect(oneWay1.url).toMatch(/^wss:\/\//);
		expect(oneWay2.url).toMatch(/^ws:\/\//);
	});

	it("Gives consumers pre-parsed versions of message events", () => {
		let publisher!: MockPublisher;
		const oneWay = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket, pub] = createMockWebSocket(url, protocols);
				publisher = pub;
				return socket;
			},
		});

		const onMessage = jest.fn();
		oneWay.addEventListener("message", onMessage);

		const payload = {
			value: 5,
			cool: "yes",
		};
		const event = new MessageEvent("message", {
			data: JSON.stringify(payload),
		});

		publisher.publishMessage(event);
		expect(onMessage).toHaveBeenCalledWith({
			sourceEvent: event,
			parsedMessage: payload,
			parseError: undefined,
		});
	});

	it("Exposes parsing error if message payload could not be parsed as JSON", () => {
		let publisher!: MockPublisher;
		const oneWay = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket, pub] = createMockWebSocket(url, protocols);
				publisher = pub;
				return socket;
			},
		});

		const onMessage = jest.fn();
		oneWay.addEventListener("message", onMessage);

		const payload = "definitely not valid JSON";
		const event = new MessageEvent("message", {
			data: payload,
		});
		publisher.publishMessage(event);

		const arg: OneWayMessageEvent<never> = onMessage.mock.lastCall[0];
		expect(arg.sourceEvent).toEqual(event);
		expect(arg.parsedMessage).toEqual(undefined);
		expect(arg.parseError).toBeInstanceOf(Error);
	});

	it("Passes all search param values through Websocket URL", () => {
		const input1: Record<string, string> = {
			cool: "yeah",
			yeah: "cool",
			blah: "5",
		};
		const oneWay1 = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket] = createMockWebSocket(url, protocols);
				return socket;
			},
			searchParams: input1,
			location: {
				protocol: "https:",
				host: "www.blah.com",
			},
		});
		let [base, params] = oneWay1.url.split("?");
		expect(base).toBe("wss://www.blah.com/api/v2/blah");
		for (const [key, value] of Object.entries(input1)) {
			expect(params).toContain(`${key}=${value}`);
		}

		const input2 = new URLSearchParams(input1);
		const oneWay2 = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket] = createMockWebSocket(url, protocols);
				return socket;
			},
			searchParams: input2,
			location: {
				protocol: "https:",
				host: "www.blah.com",
			},
		});
		[base, params] = oneWay2.url.split("?");
		expect(base).toBe("wss://www.blah.com/api/v2/blah");
		for (const [key, value] of Object.entries(input2)) {
			expect(params).toContain(`${key}=${value}`);
		}

		const oneWay3 = new OneWayWebSocket({
			apiRoute: dummyRoute,
			websocketInit: (url, protocols) => {
				const [socket] = createMockWebSocket(url, protocols);
				return socket;
			},
			searchParams: undefined,
			location: {
				protocol: "https:",
				host: "www.blah.com",
			},
		});
		[base, params] = oneWay3.url.split("?");
		expect(base).toBe("wss://www.blah.com/api/v2/blah");
		expect(params).toBe(undefined);
	});
});
