import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatMCPApp } from "#/api/typesGenerated";
import { MockChatMCPApp } from "#/testHelpers/chatEntities";
import { attachMcpAppBridge, type McpAppBridgeHandle } from "./mcpAppBridge";

const mockMcpApp: ChatMCPApp = {
	...MockChatMCPApp,
	tool_input: { count: 2 },
	tool_result: {
		content: [{ type: "text", text: "Rolled 3, 5" }],
		structuredContent: { rolls: [3, 5], total: 8 },
	},
};

type JsonRpcMessage = {
	jsonrpc: "2.0";
	id?: number;
	method?: string;
	params?: Record<string, unknown>;
	result?: Record<string, unknown>;
	error?: { code: number; message: string };
};

// A fake view: it posts JSON-RPC messages to the host as the iframe's
// window and records what the host sends back.
function mountView() {
	const frame = document.createElement("iframe");
	document.body.append(frame);
	const view = frame.contentWindow;
	if (!view) {
		throw new Error("jsdom gave the iframe no window");
	}
	const received: JsonRpcMessage[] = [];
	view.addEventListener("message", (event) => {
		received.push(event.data);
	});
	const send = (message: Omit<JsonRpcMessage, "jsonrpc">) => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { jsonrpc: "2.0", ...message },
				source: view,
			}),
		);
	};
	const waitFor = (match: (message: JsonRpcMessage) => boolean) =>
		vi.waitFor(() => {
			const message = received.find(match);
			expect(message).toBeDefined();
			return message as JsonRpcMessage;
		});
	return { frame, received, send, waitFor };
}

describe("attachMcpAppBridge", () => {
	let handle: McpAppBridgeHandle | undefined;
	afterEach(() => {
		handle?.close();
		handle = undefined;
		document.body.replaceChildren();
		vi.restoreAllMocks();
	});

	const initialize = async (view: ReturnType<typeof mountView>) => {
		view.send({
			id: 1,
			method: "ui/initialize",
			params: {
				protocolVersion: "2026-01-26",
				appInfo: { name: "dice", version: "1.0.0" },
				appCapabilities: {},
			},
		});
		return view.waitFor((message) => message.id === 1);
	};

	it("sends the tool data only after the view initialized", async () => {
		const view = mountView();
		handle = attachMcpAppBridge(view.frame, {
			app: mockMcpApp,
			theme: "dark",
			hostVersion: "v2.0.0",
			callTool: vi.fn(),
			sendMessage: vi.fn(),
		});

		const response = await initialize(view);
		expect(response.result?.hostContext).toMatchObject({ theme: "dark" });
		expect(
			view.received.some((message) => message.method?.includes("tool-")),
		).toBe(false);

		view.send({ method: "ui/notifications/initialized", params: {} });
		const input = await view.waitFor(
			(message) => message.method === "ui/notifications/tool-input",
		);
		const result = await view.waitFor(
			(message) => message.method === "ui/notifications/tool-result",
		);
		expect(input.params).toEqual({ arguments: { count: 2 } });
		expect(result.params).toMatchObject({
			structuredContent: { rolls: [3, 5], total: 8 },
		});
	});

	it("runs view requests through the host callbacks", async () => {
		const view = mountView();
		const callTool = vi.fn().mockResolvedValue({
			content: [{ type: "text", text: "Rolled 6" }],
		});
		const sendMessage = vi.fn().mockResolvedValue(undefined);
		const openWindow = vi.spyOn(window, "open").mockReturnValue(null);
		handle = attachMcpAppBridge(view.frame, {
			app: mockMcpApp,
			theme: "light",
			hostVersion: "v2.0.0",
			callTool,
			sendMessage,
		});
		await initialize(view);
		view.send({ method: "ui/notifications/initialized", params: {} });

		view.send({
			id: 2,
			method: "tools/call",
			params: { name: "roll_dice", arguments: { count: 1 } },
		});
		const toolResponse = await view.waitFor((message) => message.id === 2);
		expect(callTool).toHaveBeenCalledWith("roll_dice", { count: 1 });
		expect(toolResponse.result).toMatchObject({
			content: [{ type: "text", text: "Rolled 6" }],
		});

		view.send({
			id: 3,
			method: "ui/message",
			params: {
				role: "user",
				content: [{ type: "text", text: "What are the odds?" }],
			},
		});
		const messageResponse = await view.waitFor((message) => message.id === 3);
		expect(messageResponse.result).toEqual({});
		expect(sendMessage).toHaveBeenCalledWith("What are the odds?");

		view.send({
			id: 4,
			method: "ui/open-link",
			params: { url: "http://example.com" },
		});
		const linkResponse = await view.waitFor((message) => message.id === 4);
		expect(linkResponse.result).toEqual({ isError: true });
		expect(openWindow).not.toHaveBeenCalled();
	});

	it("refuses view messages that the chat must not send", async () => {
		const view = mountView();
		const sendMessage = vi
			.fn()
			.mockRejectedValue(new Error("The chat cannot take a message now."));
		handle = attachMcpAppBridge(view.frame, {
			app: mockMcpApp,
			theme: "light",
			hostVersion: "v2.0.0",
			callTool: vi.fn(),
			sendMessage,
		});
		await initialize(view);
		view.send({ method: "ui/notifications/initialized", params: {} });

		view.send({
			id: 2,
			method: "ui/message",
			params: { role: "user", content: [{ type: "text", text: " /clear" }] },
		});
		const slashResponse = await view.waitFor((message) => message.id === 2);
		expect(slashResponse.result).toEqual({ isError: true });
		expect(sendMessage).not.toHaveBeenCalled();

		view.send({
			id: 3,
			method: "ui/message",
			params: { role: "user", content: [{ type: "text", text: "Roll again" }] },
		});
		const refusedResponse = await view.waitFor((message) => message.id === 3);
		expect(refusedResponse.result).toEqual({ isError: true });
		expect(sendMessage).toHaveBeenCalledWith("Roll again");
	});

	it("ignores messages from other windows", async () => {
		const view = mountView();
		handle = attachMcpAppBridge(view.frame, {
			app: mockMcpApp,
			theme: "light",
			hostVersion: "v2.0.0",
			callTool: vi.fn(),
			sendMessage: vi.fn(),
		});

		window.dispatchEvent(
			new MessageEvent("message", {
				data: {
					jsonrpc: "2.0",
					id: 9,
					method: "ui/initialize",
					params: {
						protocolVersion: "2026-01-26",
						appInfo: { name: "other", version: "1.0.0" },
						appCapabilities: {},
					},
				},
				source: window,
			}),
		);
		// A real initialize from the view still gets an answer, so the
		// bridge listened while it dropped the other message.
		await initialize(view);
		expect(view.received.some((message) => message.id === 9)).toBe(false);
	});
});
