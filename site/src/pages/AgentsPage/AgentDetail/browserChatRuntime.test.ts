import type * as TypesGen from "api/typesGenerated";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { describe, expect, it, vi } from "vitest";
import { createBrowserChatRuntime } from "./browserChatRuntime";

type BrowserChatRuntimeDeps = NonNullable<
	Parameters<typeof createBrowserChatRuntime>[0]
>;
type BrowserChatSocket = ReturnType<BrowserChatRuntimeDeps["watchChat"]>;

type MockSocket = {
	addEventListener: (
		event: string,
		handler: (payload: OneWayMessageEvent<TypesGen.ServerSentEvent>) => void,
	) => void;
	close: () => void;
	emitMessage: (payload: OneWayMessageEvent<TypesGen.ServerSentEvent>) => void;
};

const asBrowserChatSocket = (socket: MockSocket): BrowserChatSocket =>
	socket as unknown as BrowserChatSocket;

const asMockSocket = (socket: BrowserChatSocket): MockSocket =>
	socket as unknown as MockSocket;

const createChat = (overrides: Partial<TypesGen.Chat> = {}): TypesGen.Chat => ({
	id: "chat-1",
	owner_id: "owner-1",
	workspace_id: "workspace-1",
	parent_chat_id: undefined,
	root_chat_id: undefined,
	last_model_config_id: "model-1",
	title: "Chat 1",
	status: "completed",
	last_error: null,
	diff_status: undefined,
	created_at: "2024-01-01T00:00:00.000Z",
	updated_at: "2024-01-01T00:00:00.000Z",
	archived: false,
	...overrides,
});

const createChatMessage = (
	overrides: Partial<TypesGen.ChatMessage> = {},
): TypesGen.ChatMessage => ({
	id: 1,
	chat_id: "chat-1",
	created_by: "owner-1",
	model_config_id: "model-1",
	created_at: "2024-01-01T00:00:00.000Z",
	role: "assistant",
	content: [],
	usage: undefined,
	...overrides,
});

const createChatDetail = (
	overrides: Partial<TypesGen.ChatWithMessages> = {},
): TypesGen.ChatWithMessages => ({
	chat: createChat(),
	messages: [],
	queued_messages: [],
	...overrides,
});

const createSendMessageResult = (
	overrides: Partial<TypesGen.CreateChatMessageResponse> = {},
): TypesGen.CreateChatMessageResponse => ({
	message: createChatMessage({ role: "user" }),
	queued_message: undefined,
	queued: false,
	...overrides,
});

const createMockSocket = (): MockSocket => {
	const messageListeners = new Set<
		(payload: OneWayMessageEvent<TypesGen.ServerSentEvent>) => void
	>();

	return {
		addEventListener(event, handler) {
			if (event === "message") {
				messageListeners.add(handler);
			}
		},
		close: vi.fn(),
		emitMessage(payload) {
			for (const listener of messageListeners) {
				listener(payload);
			}
		},
	};
};

const createReconnectHarness = () => {
	const sockets: MockSocket[] = [];
	let connect: (() => BrowserChatSocket) | undefined;
	const dispose = vi.fn();
	const rawCreateReconnectingWebSocket = vi.fn(
		(options: { connect: () => BrowserChatSocket }) => {
			connect = options.connect;
			sockets.push(asMockSocket(options.connect()));
			return dispose;
		},
	);

	return {
		sockets,
		dispose,
		createReconnectingWebSocket:
			rawCreateReconnectingWebSocket as unknown as BrowserChatRuntimeDeps["createReconnectingWebSocket"],
		reconnect() {
			if (!connect) {
				throw new Error("Reconnect requested before connect().");
			}
			sockets.push(asMockSocket(connect()));
			return sockets[sockets.length - 1];
		},
	};
};

const createDeps = (
	overrides: Partial<BrowserChatRuntimeDeps> = {},
): BrowserChatRuntimeDeps => ({
	getChats: vi.fn(async () => []),
	getChat: vi.fn(async () => createChatDetail()),
	createChatMessage: vi.fn(async () => createSendMessageResult()),
	getChatModels: vi.fn(async () => ({ providers: [] })),
	watchChat: vi.fn(() =>
		asBrowserChatSocket(createMockSocket()),
	) as unknown as BrowserChatRuntimeDeps["watchChat"],
	createReconnectingWebSocket: vi.fn(() =>
		vi.fn(),
	) as unknown as BrowserChatRuntimeDeps["createReconnectingWebSocket"],
	...overrides,
});

const createDataMessage = (
	data: unknown,
): OneWayMessageEvent<TypesGen.ServerSentEvent> => ({
	sourceEvent: new MessageEvent("message", { data: JSON.stringify(data) }),
	parseError: undefined,
	parsedMessage: { type: "data", data },
});

const createParseErrorMessage =
	(): OneWayMessageEvent<TypesGen.ServerSentEvent> => ({
		sourceEvent: new MessageEvent("message", { data: "not-json" }),
		parseError: new Error("parse failed"),
		parsedMessage: undefined,
	});

describe("createBrowserChatRuntime", () => {
	it("calls getChats with list params and post-filters archived chats", async () => {
		const chats = [
			createChat({ id: "chat-1", archived: false }),
			createChat({ id: "chat-2", archived: true }),
		];
		const deps = createDeps({
			getChats: vi.fn(async () => chats),
		});
		const runtime = createBrowserChatRuntime(deps);

		await expect(
			runtime.listChats({ archived: true, limit: 10, offset: 20 }),
		).resolves.toEqual([chats[1]]);
		expect(deps.getChats).toHaveBeenCalledWith({ limit: 10, offset: 20 });
	});

	it("passes getChat through to the site API", async () => {
		const detail = createChatDetail({
			messages: [createChatMessage({ id: 7 })],
		});
		const deps = createDeps({
			getChat: vi.fn(async () => detail),
		});
		const runtime = createBrowserChatRuntime(deps);

		await expect(runtime.getChat("chat-7")).resolves.toBe(detail);
		expect(deps.getChat).toHaveBeenCalledWith("chat-7");
	});

	it("maps sendMessage input to the REST API and makes parentMessageId visible", async () => {
		const result = createSendMessageResult({ queued: true });
		const deps = createDeps({
			createChatMessage: vi.fn(async () => result),
		});
		const runtime = createBrowserChatRuntime(deps);
		const warnSpy = vi
			.spyOn(console, "warn")
			.mockImplementation(() => undefined);

		await expect(
			runtime.sendMessage({
				chatId: "chat-1",
				message: "Hello",
				model: "model-2",
				parentMessageId: 99,
			}),
		).resolves.toBe(result);
		expect(deps.createChatMessage).toHaveBeenCalledWith("chat-1", {
			content: [{ type: "text", text: "Hello" }],
			model_config_id: "model-2",
		});
		expect(warnSpy).toHaveBeenCalledWith(
			"browserChatRuntime.sendMessage received a parentMessageId, but the browser chat API does not support threaded replies yet.",
		);

		warnSpy.mockRestore();
	});

	it("flattens available model providers into ChatModelOption values", async () => {
		const chatModelsResponse: TypesGen.ChatModelsResponse = {
			providers: [
				{
					provider: "openai",
					available: true,
					models: [
						{
							id: "model-1",
							provider: "openai",
							model: "gpt-4o",
							display_name: "GPT-4o",
						},
					],
				},
				{
					provider: "anthropic",
					available: false,
					unavailable_reason: "missing_api_key",
					models: [
						{
							id: "model-2",
							provider: "anthropic",
							model: "claude-sonnet",
							display_name: "Claude Sonnet",
						},
					],
				},
			],
		};
		const deps = createDeps({
			getChatModels: vi.fn(async () => chatModelsResponse),
		});
		const runtime = createBrowserChatRuntime(deps);

		await expect(runtime.listModels()).resolves.toEqual([
			{
				id: "model-1",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT-4o",
			},
		]);
	});

	it("forwards single stream events", () => {
		const reconnect = createReconnectHarness();
		const socket = createMockSocket();
		const watchChatSpy = vi.fn(() => asBrowserChatSocket(socket));
		const deps = createDeps({
			watchChat: watchChatSpy as unknown as BrowserChatRuntimeDeps["watchChat"],
			createReconnectingWebSocket: reconnect.createReconnectingWebSocket,
		});
		const runtime = createBrowserChatRuntime(deps);
		const onEvent = vi.fn();

		runtime.subscribeToChat({ chatId: "chat-1" }, onEvent);
		socket.emitMessage(
			createDataMessage({
				type: "status",
				chat_id: "chat-1",
				status: { status: "running" },
			}),
		);

		expect(watchChatSpy).toHaveBeenCalledWith("chat-1", undefined);
		expect(onEvent).toHaveBeenCalledWith({
			type: "status",
			chat_id: "chat-1",
			status: { status: "running" },
		});
	});

	it("forwards batched stream events", () => {
		const reconnect = createReconnectHarness();
		const socket = createMockSocket();
		const watchChatSpy = vi.fn(() => asBrowserChatSocket(socket));
		const deps = createDeps({
			watchChat: watchChatSpy as unknown as BrowserChatRuntimeDeps["watchChat"],
			createReconnectingWebSocket: reconnect.createReconnectingWebSocket,
		});
		const runtime = createBrowserChatRuntime(deps);
		const onEvent = vi.fn();

		runtime.subscribeToChat({ chatId: "chat-1" }, onEvent);
		socket.emitMessage(
			createDataMessage([
				{
					type: "queue_update",
					chat_id: "chat-1",
					queued_messages: [],
				},
				{
					type: "retry",
					chat_id: "chat-1",
					retry: { attempt: 2, error: "temporary" },
				},
			]),
		);

		expect(watchChatSpy).toHaveBeenCalledWith("chat-1", undefined);
		expect(onEvent).toHaveBeenNthCalledWith(1, {
			type: "queue_update",
			chat_id: "chat-1",
			queued_messages: [],
		});
		expect(onEvent).toHaveBeenNthCalledWith(2, {
			type: "retry",
			chat_id: "chat-1",
			retry: { attempt: 2, error: "temporary" },
		});
	});

	it("ignores parse errors and non-data frames", () => {
		const reconnect = createReconnectHarness();
		const socket = createMockSocket();
		const watchChatSpy = vi.fn(() => asBrowserChatSocket(socket));
		const deps = createDeps({
			watchChat: watchChatSpy as unknown as BrowserChatRuntimeDeps["watchChat"],
			createReconnectingWebSocket: reconnect.createReconnectingWebSocket,
		});
		const runtime = createBrowserChatRuntime(deps);
		const onEvent = vi.fn();

		runtime.subscribeToChat({ chatId: "chat-1" }, onEvent);
		socket.emitMessage(createParseErrorMessage());
		socket.emitMessage({
			sourceEvent: new MessageEvent("message", { data: JSON.stringify({}) }),
			parseError: undefined,
			parsedMessage: { type: "ping", data: {} },
		});

		expect(watchChatSpy).toHaveBeenCalledWith("chat-1", undefined);
		expect(onEvent).not.toHaveBeenCalled();
	});

	it("reconnects with the latest durable message ID after message events", () => {
		const reconnect = createReconnectHarness();
		const sockets = [createMockSocket(), createMockSocket()];
		const watchChatSpy = vi.fn(() =>
			asBrowserChatSocket(sockets.shift() ?? createMockSocket()),
		);
		const deps = createDeps({
			watchChat: watchChatSpy as unknown as BrowserChatRuntimeDeps["watchChat"],
			createReconnectingWebSocket: reconnect.createReconnectingWebSocket,
		});
		const runtime = createBrowserChatRuntime(deps);
		const onEvent = vi.fn();

		runtime.subscribeToChat({ chatId: "chat-1", afterMessageId: 5 }, onEvent);
		reconnect.sockets[0]?.emitMessage(
			createDataMessage({
				type: "message",
				chat_id: "chat-1",
				message: createChatMessage({ id: 11 }),
			}),
		);

		reconnect.reconnect();

		expect(watchChatSpy).toHaveBeenNthCalledWith(1, "chat-1", 5);
		expect(watchChatSpy).toHaveBeenNthCalledWith(2, "chat-1", 11);
	});

	it("stops emitting and tears down the reconnecting socket after dispose", () => {
		const reconnect = createReconnectHarness();
		const socket = createMockSocket();
		const watchChatSpy = vi.fn(() => asBrowserChatSocket(socket));
		const deps = createDeps({
			watchChat: watchChatSpy as unknown as BrowserChatRuntimeDeps["watchChat"],
			createReconnectingWebSocket: reconnect.createReconnectingWebSocket,
		});
		const runtime = createBrowserChatRuntime(deps);
		const onEvent = vi.fn();

		const subscription = runtime.subscribeToChat({ chatId: "chat-1" }, onEvent);
		subscription.dispose();
		socket.emitMessage(
			createDataMessage({
				type: "status",
				chat_id: "chat-1",
				status: { status: "completed" },
			}),
		);

		expect(watchChatSpy).toHaveBeenCalledWith("chat-1", undefined);
		expect(reconnect.dispose).toHaveBeenCalledTimes(1);
		expect(onEvent).not.toHaveBeenCalled();
	});
});
