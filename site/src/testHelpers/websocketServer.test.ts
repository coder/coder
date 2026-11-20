import { mockWebSocketServer } from "./websocketServer";

const openSocket = async (url: string) => {
	const socket = new WebSocket(url);
	await new Promise((resolve) => socket.addEventListener("open", resolve));
	return socket;
};

const nextData = (socket: WebSocket) =>
	new Promise<unknown>((resolve) =>
		socket.addEventListener("message", (event) => resolve(event.data), {
			once: true,
		}),
	);

describe(mockWebSocketServer.name, () => {
	const baseUrl = `ws://${location.host}/api/v2/test`;

	it("tracks each connection with its URL and path parameters", async () => {
		const mockServer = mockWebSocketServer("/api/v2/test/:id");
		const first = await openSocket(`${baseUrl}/one?mode=a`);
		const second = await openSocket(`${baseUrl}/two`);

		const [firstConnection, secondConnection] = await Promise.all([
			mockServer.connection(0),
			mockServer.connection(1),
		]);
		expect(firstConnection.params.id).toBe("one");
		expect(firstConnection.url.searchParams.get("mode")).toBe("a");
		expect(secondConnection.params.id).toBe("two");

		first.send("to first");
		second.send("to second");
		expect(await mockServer.nextMessage()).toBe("to first");
		expect(await mockServer.nextMessage()).toBe("to second");
		expect(firstConnection.messages).toEqual(["to first"]);
		expect(secondConnection.messages).toEqual(["to second"]);
	});

	it("waits for connections and messages that have not arrived yet", async () => {
		const mockServer = mockWebSocketServer("/api/v2/test");
		const connection = mockServer.connection();
		const message = mockServer.nextMessage();

		const socket = await openSocket(baseUrl);
		socket.send("hello");

		expect((await connection).messages).toEqual(["hello"]);
		expect(await message).toBe("hello");
	});

	it("only matches the path on the page's origin", async () => {
		const mockServer = mockWebSocketServer("/api/v2/test");
		await openSocket("ws://example.com/api/v2/test");
		await openSocket(`ws://${location.host}/api/v2/other`);
		await openSocket(baseUrl);

		await mockServer.connection();
		expect(mockServer.connections).toHaveLength(1);
	});

	it("sends to clients and closes connections", async () => {
		const mockServer = mockWebSocketServer("/api/v2/test");
		const socket = await openSocket(baseUrl);
		const connection = await mockServer.connection();

		const received = nextData(socket);
		mockServer.send("broadcast");
		expect(await received).toBe("broadcast");

		const closed = new Promise<CloseEvent>((resolve) =>
			socket.addEventListener("close", resolve),
		);
		connection.close(4000, "done");
		const event = await closed;
		expect(event.code).toBe(4000);
		expect(socket.readyState).toBe(WebSocket.CLOSED);
	});
});
