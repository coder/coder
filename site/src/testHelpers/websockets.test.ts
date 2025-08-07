import { createMockWebSocket } from "./websockets";

describe(createMockWebSocket.name, () => {
	it("Throws if URL does not have ws:// or wss:// protocols", () => {
		const urls: readonly string[] = [
			"http://www.dog.ceo/roll-over",
			"https://www.dog.ceo/roll-over",
		];
		for (const url of urls) {
			expect(() => {
				void createMockWebSocket(url);
			}).toThrow("URL must start with ws:// or wss://");
		}
	});

	it("Sends events from server to socket", () => {
		const [socket, server] = createMockWebSocket("wss://www.dog.ceo/shake");

		const onOpen = jest.fn();
		const onError = jest.fn();
		const onMessage = jest.fn();
		const onClose = jest.fn();

		socket.addEventListener("open", onOpen);
		socket.addEventListener("error", onError);
		socket.addEventListener("message", onMessage);
		socket.addEventListener("close", onClose);

		const openEvent = new Event("open");
		const errorEvent = new Event("error");
		const messageEvent = new MessageEvent<string>("message");
		const closeEvent = new CloseEvent("close");

		server.publishOpen(openEvent);
		server.publishError(errorEvent);
		server.publishMessage(messageEvent);
		server.publishClose(closeEvent);

		expect(onOpen).toHaveBeenCalledTimes(1);
		expect(onOpen).toHaveBeenCalledWith(openEvent);

		expect(onError).toHaveBeenCalledTimes(1);
		expect(onError).toHaveBeenCalledWith(errorEvent);

		expect(onMessage).toHaveBeenCalledTimes(1);
		expect(onMessage).toHaveBeenCalledWith(messageEvent);

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledWith(closeEvent);
	});

	it("Sends JSON data to the socket for message events", () => {
		const [socket, server] = createMockWebSocket("wss://www.dog.ceo/wag");
		const onMessage = jest.fn();

		// Could type this as a special JSON type, but unknown is good enough,
		// since any invalid values will throw in the test case
		const jsonData: readonly unknown[] = [
			"blah",
			42,
			true,
			false,
			null,
			{},
			[],
			[{ value: "blah" }, { value: "guh" }, { value: "huh" }],
			{
				name: "Hershel Layton",
				age: 40,
				profession: "Puzzle Solver",
				sadBackstory: true,
				greatVideoGames: true,
			},
		];
		for (const jd of jsonData) {
			socket.addEventListener("message", onMessage);
			server.publishMessage(
				new MessageEvent("message", { data: JSON.stringify(jd) }),
			);

			expect(onMessage).toHaveBeenCalledTimes(1);
			expect(onMessage).toHaveBeenCalledWith(
				new MessageEvent("message", { data: JSON.stringify(jd) }),
			);

			socket.removeEventListener("message", onMessage);
			onMessage.mockClear();
		}
	});

	it("Only registers each socket event handler once", () => {
		const [socket, server] = createMockWebSocket("wss://www.dog.ceo/borf");

		const onOpen = jest.fn();
		const onError = jest.fn();
		const onMessage = jest.fn();
		const onClose = jest.fn();

		// Do it once
		socket.addEventListener("open", onOpen);
		socket.addEventListener("error", onError);
		socket.addEventListener("message", onMessage);
		socket.addEventListener("close", onClose);

		// Do it again with the exact same functions
		socket.addEventListener("open", onOpen);
		socket.addEventListener("error", onError);
		socket.addEventListener("message", onMessage);
		socket.addEventListener("close", onClose);

		server.publishOpen(new Event("open"));
		server.publishError(new Event("error"));
		server.publishMessage(new MessageEvent<string>("message"));
		server.publishClose(new CloseEvent("close"));

		expect(onOpen).toHaveBeenCalledTimes(1);
		expect(onError).toHaveBeenCalledTimes(1);
		expect(onMessage).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it("Lets a socket unsubscribe to event types", () => {
		const [socket, server] = createMockWebSocket("wss://www.dog.ceo/zoomies");

		const onOpen = jest.fn();
		const onError = jest.fn();
		const onMessage = jest.fn();
		const onClose = jest.fn();

		socket.addEventListener("open", onOpen);
		socket.addEventListener("error", onError);
		socket.addEventListener("message", onMessage);
		socket.addEventListener("close", onClose);

		socket.removeEventListener("open", onOpen);
		socket.removeEventListener("error", onError);
		socket.removeEventListener("message", onMessage);
		socket.removeEventListener("close", onClose);

		server.publishOpen(new Event("open"));
		server.publishError(new Event("error"));
		server.publishMessage(new MessageEvent<string>("message"));
		server.publishClose(new CloseEvent("close"));

		expect(onOpen).not.toHaveBeenCalled();
		expect(onError).not.toHaveBeenCalled();
		expect(onMessage).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
	});

	it("Renders socket inert after being closed", () => {
		const [socket, server] = createMockWebSocket("wss://www.dog.ceo/woof");
		expect(server.isConnectionOpen).toBe(true);

		const onMessage = jest.fn();
		socket.addEventListener("message", onMessage);

		socket.close();
		expect(server.isConnectionOpen).toBe(false);

		server.publishMessage(new MessageEvent<string>("message"));
		expect(onMessage).not.toHaveBeenCalled();
	});

	it("Tracks arguments sent by the mock socket", () => {
		const [socket, server] = createMockWebSocket("wss://www.dog.ceo/wan-wan");
		const data = JSON.stringify({
			famousDogs: [
				"snoopy",
				"clifford",
				"lassie",
				"beethoven",
				"courage the cowardly dog",
			],
		});

		socket.send(data);
		expect(server.clientSentData).toHaveLength(1);
		expect(server.clientSentData).toEqual([data]);

		socket.close();
		socket.send(data);
		expect(server.clientSentData).toHaveLength(1);
		expect(server.clientSentData).toEqual([data]);
	});
});
