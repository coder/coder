/**
 * @file Mocks WebSocket servers with MSW, which intercepts the WebSockets the
 * code under test opens. Handlers are registered on the shared MSW server, so
 * they are reset after each test.
 */
import { type WebSocketData, ws } from "msw";
import { server } from "./server";

type MockWebSocketConnection = Readonly<{
	/** The URL the client connected to, including its query parameters. */
	url: URL;
	/** Path parameters matched from the URL pattern. */
	params: Readonly<Record<string, string | readonly string[] | undefined>>;
	/** Messages received from the client on this connection, oldest first. */
	messages: readonly WebSocketData[];
	send: (data: WebSocketData) => void;
	close: (code?: number, reason?: string) => void;
}>;

type MockWebSocketServer = Readonly<{
	/** Every connection made to the server so far, oldest first. */
	connections: readonly MockWebSocketConnection[];
	/** Resolves with the connection at `index` once the client has made it. */
	connection: (index?: number) => Promise<MockWebSocketConnection>;
	/**
	 * Resolves with the oldest message from any connection that has not been
	 * returned by a previous call, waiting for one if there is none yet.
	 */
	nextMessage: () => Promise<WebSocketData>;
	/** Sends data to every connection. */
	send: (data: WebSocketData) => void;
}>;

/**
 * Mocks a WebSocket server for every URL matching `url`. The pattern is matched
 * like an MSW HTTP handler: it may contain path parameters (`:id`) and
 * wildcards (`*`), and the query string is ignored.
 */
export function mockWebSocketServer(url: string | RegExp): MockWebSocketServer {
	const connections: MockWebSocketConnection[] = [];
	const connectionWaiters = new Map<number, Array<() => void>>();
	const messages: WebSocketData[] = [];
	let nextMessageIndex = 0;
	let messageWaiters: Array<() => void> = [];

	server.use(
		ws.link(url).addEventListener("connection", ({ client, params }) => {
			const connectionMessages: WebSocketData[] = [];
			client.addEventListener("message", (event) => {
				connectionMessages.push(event.data);
				messages.push(event.data);
				const waiters = messageWaiters;
				messageWaiters = [];
				for (const resolve of waiters) {
					resolve();
				}
			});

			const index = connections.length;
			connections.push({
				url: client.url,
				params,
				messages: connectionMessages,
				send: (data) => client.send(data),
				close: (code, reason) => client.close(code, reason),
			});
			for (const resolve of connectionWaiters.get(index) ?? []) {
				resolve();
			}
			connectionWaiters.delete(index);
		}),
	);

	return {
		connections,
		connection: async (index = 0) => {
			if (!connections[index]) {
				await new Promise<void>((resolve) => {
					connectionWaiters.set(index, [
						...(connectionWaiters.get(index) ?? []),
						resolve,
					]);
				});
			}
			return connections[index];
		},
		nextMessage: async () => {
			const index = nextMessageIndex++;
			while (messages.length <= index) {
				await new Promise<void>((resolve) => {
					messageWaiters.push(resolve);
				});
			}
			return messages[index];
		},
		send: (data) => {
			for (const connection of connections) {
				connection.send(data);
			}
		},
	};
}
