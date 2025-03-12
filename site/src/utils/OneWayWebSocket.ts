/**
 * @file A wrapper over WebSockets that (1) enforces one-way communication, and
 * (2) supports automatically parsing JSON messages as they come in.
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
 * browsers implement at least some degree of multiplexing for them.
 */

// Not bothering with trying to borrow methods from the base WebSocket type
// because it's already a mess of inheritance and generics, and we're going to
// have to add a few more
type WebSocketEventType = "close" | "error" | "message" | "open";

type OneWayMessageEvent<TData> = Readonly<
	| {
			sourceEvent: MessageEvent<string>;
			parsedMessage: TData;
			parseError: undefined;
	  }
	| {
			sourceEvent: MessageEvent<string>;
			parsedMessage: undefined;
			parseError: Error;
	  }
>;

type OneWayEventPayloadMap<TData> = {
	close: CloseEvent;
	error: Event;
	message: OneWayMessageEvent<TData>;
	open: Event;
};

type WebSocketMessageCallback = (payload: MessageEvent<string>) => void;

type OneWayEventCallback<TData, TEvent extends WebSocketEventType> = (
	payload: OneWayEventPayloadMap<TData>[TEvent],
) => void;

type OneWayWebSocketInit = Readonly<{
	apiRoute: string;
	location?: Location;
	protocols?: string | string[];
}>;

interface OneWayWebSocketApi<TData> {
	addEventListener: <TEvent extends WebSocketEventType>(
		eventType: TEvent,
		callback: OneWayEventCallback<TData, TEvent>,
	) => void;

	removeEventListener: <TEvent extends WebSocketEventType>(
		eventType: TEvent,
		callback: OneWayEventCallback<TData, TEvent>,
	) => void;

	close: (closeCode?: number, reason?: string) => void;
}

export class OneWayWebSocket<TData = unknown>
	implements OneWayWebSocketApi<TData>
{
	#socket: WebSocket;
	#messageCallbackWrappers = new Map<
		OneWayEventCallback<TData, "message">,
		WebSocketMessageCallback
	>();

	constructor(init: OneWayWebSocketInit) {
		const { apiRoute, protocols, location = window.location } = init;
		if (apiRoute.at(0) !== "/") {
			throw new Error(`API route ${apiRoute} does not begin with a space`);
		}

		const protocol = location.protocol === "https:" ? "wss:" : "ws:";
		const url = `${protocol}//${location.host}${apiRoute}`;
		this.#socket = new WebSocket(url, protocols);
	}

	addEventListener<TEvent extends WebSocketEventType>(
		event: TEvent,
		callback: OneWayEventCallback<TData, TEvent>,
	): void {
		// Not happy about all the type assertions, but there are some nasty
		// type contravariance issues if you try to resolve the function types
		// properly. This is actually the lesser of two evils
		const looseCallback = callback as OneWayEventCallback<
			TData,
			WebSocketEventType
		>;

		if (this.#messageCallbackWrappers.has(looseCallback)) {
			return;
		}
		if (event !== "message") {
			this.#socket.addEventListener(event, looseCallback);
			return;
		}

		const wrapped = (event: MessageEvent<string>): void => {
			const messageCallback = looseCallback as OneWayEventCallback<
				TData,
				"message"
			>;

			try {
				const message = JSON.parse(event.data) as TData;
				messageCallback({
					sourceEvent: event,
					parseError: undefined,
					parsedMessage: message,
				});
			} catch (err) {
				messageCallback({
					sourceEvent: event,
					parseError: err as Error,
					parsedMessage: undefined,
				});
			}
		};

		this.#socket.addEventListener(event as "message", wrapped);
		this.#messageCallbackWrappers.set(looseCallback, wrapped);
	}

	removeEventListener<TEvent extends WebSocketEventType>(
		event: TEvent,
		callback: OneWayEventCallback<TData, TEvent>,
	): void {
		const looseCallback = callback as OneWayEventCallback<
			TData,
			WebSocketEventType
		>;

		if (!this.#messageCallbackWrappers.has(looseCallback)) {
			return;
		}
		if (event !== "message") {
			this.#socket.removeEventListener(event, looseCallback);
			return;
		}

		const wrapper = this.#messageCallbackWrappers.get(looseCallback);
		if (wrapper === undefined) {
			throw new Error(
				`Cannot unregister callback for event ${event}. This is likely an issue with the browser itself.`,
			);
		}

		this.#socket.removeEventListener(event as "message", wrapper);
		this.#messageCallbackWrappers.delete(looseCallback);
	}

	close(closeCode?: number, reason?: string): void {
		this.#socket.close(closeCode, reason);
	}
}
