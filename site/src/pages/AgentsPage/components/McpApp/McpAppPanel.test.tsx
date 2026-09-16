import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChat, MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { MockBuildInfo } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { createChatStore } from "../ChatConversation/chatStore";
import McpAppPanel from "./McpAppPanel";
import { type McpAppTab, mcpAppTabId } from "./mcpAppTab";
import { MCP_APP_MIME_TYPE } from "./sandboxUrl";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ buildInfo: MockBuildInfo }),
}));

const RESOURCE_URI = "ui://taskboard/board";
const TOOL_CALL_ID = "call-1";
const taskboardServer = { ...MockMCPServerConfig, display_name: "Task board" };

const tab: McpAppTab = {
	id: mcpAppTabId(taskboardServer.id, RESOURCE_URI),
	kind: "mcp_app",
	mcpServerConfigId: taskboardServer.id,
	resourceUri: RESOURCE_URI,
	toolCallId: TOOL_CALL_ID,
	label: "Task board",
};

const toolCallMessage: ChatMessage = {
	id: 1,
	chat_id: MockChat.id,
	created_at: MockChat.created_at,
	role: "assistant",
	content: [
		{
			type: "tool-call",
			tool_call_id: TOOL_CALL_ID,
			tool_name: "add_task",
			args: { title: "Write tests" },
			mcp_server_config_id: taskboardServer.id,
			mcp_app_resource_uri: RESOURCE_URI,
		},
	],
};

type Posted = { id?: unknown; result?: unknown; error?: { message: string } };

/**
 * Renders the panel against a jsdom iframe and drives the JSON-RPC channel
 * the way the sandbox proxy would: messages arrive from the iframe window
 * and origin, and host messages are captured from `postMessage`.
 */
const renderPanel = async (
	overrides: Partial<React.ComponentProps<typeof McpAppPanel>> = {},
) => {
	server.use(
		http.post(
			`/api/experimental/chats/${MockChat.id}/mcp-servers/${taskboardServer.id}/resources/read`,
			() =>
				HttpResponse.json({
					result: {
						contents: [
							{
								uri: RESOURCE_URI,
								mimeType: MCP_APP_MIME_TYPE,
								text: "<!doctype html><p>board</p>",
							},
						],
					},
				}),
		),
	);
	const store = createChatStore();
	store.replaceMessages([toolCallMessage]);
	const submitAppMessage = vi.fn<(text: string) => Promise<void>>(() =>
		Promise.resolve(),
	);
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<McpAppPanel
				chatId={MockChat.id}
				tab={tab}
				store={store}
				chatStatus="waiting"
				mcpServer={taskboardServer}
				wildcardHostname="*.apps.example.com"
				submitAppMessage={submitAppMessage}
				isClosing={false}
				onClosed={() => {}}
				{...overrides}
			/>
		</QueryClientProvider>,
	);

	const frame = await screen.findByTitle("Task board app");
	if (!(frame instanceof HTMLIFrameElement) || !frame.contentWindow) {
		throw new Error("sandbox iframe did not mount");
	}
	const frameWindow = frame.contentWindow;
	const origin = new URL(frame.src).origin;
	const posted: Posted[] = [];
	vi.spyOn(frameWindow, "postMessage").mockImplementation((message: Posted) => {
		posted.push(message);
	});

	let nextId = 1;
	const fromView = (data: unknown) => {
		act(() => {
			window.dispatchEvent(
				new MessageEvent("message", { source: frameWindow, origin, data }),
			);
		});
	};
	const request = async (method: string, params: Record<string, unknown>) => {
		const id = nextId++;
		fromView({ jsonrpc: "2.0", id, method, params });
		return {
			id,
			response: () =>
				waitFor(() => {
					const response = posted.find((message) => message.id === id);
					expect(response).toBeDefined();
					return response;
				}),
		};
	};

	fromView({
		jsonrpc: "2.0",
		method: "ui/notifications/sandbox-proxy-ready",
		params: {},
	});
	await waitFor(() => {
		expect(
			posted.some(
				(message) =>
					"method" in message &&
					message.method === "ui/notifications/sandbox-resource-ready",
			),
		).toBe(true);
	});
	await (
		await request("ui/initialize", {
			appInfo: { name: "taskboard", version: "0.0.1" },
			appCapabilities: {},
			protocolVersion: "2026-01-26",
		})
	).response();
	fromView({
		jsonrpc: "2.0",
		method: "ui/notifications/initialized",
		params: {},
	});

	return { submitAppMessage, request };
};

describe("McpAppPanel", () => {
	it("sends a ui/message only after the user confirms", async () => {
		const user = userEvent.setup();
		const { submitAppMessage, request } = await renderPanel();

		const pending = await request("ui/message", {
			role: "user",
			content: [{ type: "text", text: "Add a task for the release" }],
		});
		await screen.findByRole("dialog");
		await user.click(screen.getByRole("button", { name: "Send" }));

		expect(submitAppMessage).toHaveBeenCalledWith("Add a task for the release");
		expect(await pending.response()).toEqual(
			expect.objectContaining({ result: {} }),
		);
	});

	it("answers ui/message with an error when the user cancels", async () => {
		const user = userEvent.setup();
		const { submitAppMessage, request } = await renderPanel();

		const pending = await request("ui/message", {
			role: "user",
			content: [{ type: "text", text: "Add a task" }],
		});
		await screen.findByRole("dialog");
		await user.click(screen.getByRole("button", { name: "Cancel" }));

		const response = await pending.response();
		expect(response?.error?.message).toContain("declined");
		expect(submitAppMessage).not.toHaveBeenCalled();
	});

	it("opens a link in a new tab after the user confirms", async () => {
		const user = userEvent.setup();
		const open = vi.spyOn(window, "open").mockImplementation(() => null);
		const { request } = await renderPanel();

		const pending = await request("ui/open-link", {
			url: "https://coder.com/docs",
		});
		await screen.findByRole("dialog");
		await user.click(screen.getByRole("button", { name: "Open link" }));

		expect(open).toHaveBeenCalledWith(
			"https://coder.com/docs",
			"_blank",
			"noopener,noreferrer",
		);
		expect(await pending.response()).toEqual(
			expect.objectContaining({ result: {} }),
		);
	});

	it("rejects a second request while one awaits confirmation", async () => {
		const { request } = await renderPanel();

		await request("ui/open-link", { url: "https://coder.com/docs" });
		await screen.findByRole("dialog");
		const second = await request("ui/message", {
			role: "user",
			content: [{ type: "text", text: "Add a task" }],
		});

		const response = await second.response();
		expect(response?.error?.message).toContain("awaiting confirmation");
	});
});
