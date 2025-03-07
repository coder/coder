/**
 * @file A WebSocket that can only receive messages from the server, and cannot
 * ever send anything.
 *
 * This should ALWAYS be favored in favor of using Server-Sent Events and the
 * built-in EventSource class for doing one-way communication. SSEs have a hard
 * limitation on HTTP/1.1 and below where there is a maximum number of 6 ports
 * that can ever be used for a domain (sometimes less depending on the browser).
 * Not only is this limit shared with short-lived REST requests, but it also
 * applies across tabs and windows. So if a user opens Coder in multiple tabs,
 * there is a very real possibility that parts of the app will start to lock up
 * without it being clear why.
 *
 * WebSockets do not have this limitation, even on HTTP/1.1 – all modern
 * browsers implement at least some degree of multiplexing for them. This file
 * just provides a wrapper to make it harder to use WebSockets for two-way
 * communication by accident.
 */

// Not bothering with trying to borrow methods from the base WebSocket type
// because it's a mess of inheritance and generics.
type WebSocketEventType = "close" | "error" | "message" | "open";

type WebSocketEventPayloadMap = {
	close: CloseEvent;
	error: Event;
	message: MessageEvent<string>;
	open: Event;
};

// The generics need to apply on a per-method-invocation basis; they cannot be
// generalized to the top-level interface definition
interface OneWayWebSocketApi {
	addEventListener: <T extends WebSocketEventType>(
		eventType: T,
		callback: (payload: WebSocketEventPayloadMap[T]) => void,
	) => void;

	removeEventListener: <T extends WebSocketEventType>(
		eventType: T,
		callback: (payload: WebSocketEventPayloadMap[T]) => void,
	) => void;

	close: (closeCode?: number, reason?: string) => void;
}

// Implementing wrapper around the base WebSocket class instead of doing fancy
// compile-time type-casts so that we have more runtime assurance that we won't
// accidentally send a message from the client to the server
export class OneWayWebSocket implements OneWayWebSocketApi {
	#socket: WebSocket;

	constructor(url: string | URL, protocols?: string | string[]) {
		this.#socket = new WebSocket(url, protocols);
	}

	// Just because this is a React project, all public methods are defined as
	// arrow functions so they can be passed around as values without losing
	// their `this` context
	addEventListener = <T extends WebSocketEventType>(
		message: T,
		callback: (payload: WebSocketEventPayloadMap[T]) => void,
	): void => {
		this.#socket.addEventListener(message, callback);
	};

	removeEventListener = <T extends WebSocketEventType>(
		message: T,
		callback: (payload: WebSocketEventPayloadMap[T]) => void,
	): void => {
		this.#socket.removeEventListener(message, callback);
	};

	close = (closeCode?: number, reason?: string): void => {
		this.#socket.close(closeCode, reason);
	};
}
