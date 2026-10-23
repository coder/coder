import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatMCPApp } from "#/api/typesGenerated";
import { attachMcpAppBridge, type McpAppBridgeHandle } from "./mcpAppBridge";

const app: ChatMCPApp = {
	mcp_server_config_id: "server-1",
	tool_name: "show_dice_board",
	title: "Dice board",
	resource_uri: "ui://dice/board.html",
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
	const waitFor = async (match: (message: JsonRpcMessage) => boolean) => {
		await vi.waitFor(() => {
			expect(received.some(match)).toBe(true);
		});
		const message = received.find(match);
		if (!message) {
			throw new Error("unreachable: waitFor found no message");
		}
		return message;
	};
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
			app,
			theme: "dark",
			hostVersion: "v2.0.0",
			callTool: vi.fn(),
			insertComposerText: vi.fn(),
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
		const insertComposerText = vi.fn();
		const openWindow = vi.spyOn(window, "open").mockReturnValue(null);
		handle = attachMcpAppBridge(view.frame, {
			app,
			theme: "light",
			hostVersion: "v2.0.0",
			callTool,
			insertComposerText,
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
		await view.waitFor((message) => message.id === 3);
		expect(insertComposerText).toHaveBeenCalledWith("What are the odds?");

		view.send({
			id: 4,
			method: "ui/open-link",
			params: { url: "http://example.com" },
		});
		const linkResponse = await view.waitFor((message) => message.id === 4);
		expect(linkResponse.result).toEqual({ isError: true });
		expect(openWindow).not.toHaveBeenCalled();
	});

	it("ignores messages from other windows", async () => {
		const view = mountView();
		handle = attachMcpAppBridge(view.frame, {
			app,
			theme: "light",
			hostVersion: "v2.0.0",
			callTool: vi.fn(),
			insertComposerText: vi.fn(),
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
