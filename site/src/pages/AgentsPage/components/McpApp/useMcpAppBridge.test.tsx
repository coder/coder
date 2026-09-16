import {
	InMemoryTransport,
	type JSONRPCMessage,
	type JSONRPCRequest,
} from "@modelcontextprotocol/client";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { createChatStore } from "../ChatConversation/chatStore";
import type { BoundCall } from "./mcpAppLifecycle";
import {
	type UseMcpAppBridgeOptions,
	useMcpAppBridge,
} from "./useMcpAppBridge";

const CHAT_ID = "chat-1";
const SERVER_ID = "mcp-1";
const RESOURCE_URI = "ui://taskboard/board";
const APP_ID = `mcp_app:${SERVER_ID}:${RESOURCE_URI}`;

const isRequest = (message: JSONRPCMessage): message is JSONRPCRequest =>
	"method" in message && "id" in message;

/**
 * The view side of an in-memory transport pair: records every host
 * message and lets the test send JSON-RPC as the sandboxed app would.
 */
const createFakeView = async () => {
	const [hostTransport, viewTransport] = InMemoryTransport.createLinkedPair();
	const received: JSONRPCMessage[] = [];
	let nextId = 1;
	viewTransport.onmessage = (message) => {
		received.push(message);
	};
	await viewTransport.start();
	const waitForResponse = async (id: number) => {
		await waitFor(() => {
			expect(
				received.some((message) => "id" in message && message.id === id),
			).toBe(true);
		});
		return received.find((message) => "id" in message && message.id === id);
	};
	return {
		hostTransport,
		received,
		notify: (method: string, params: Record<string, unknown> = {}) =>
			viewTransport.send({ jsonrpc: "2.0", method, params }),
		respond: (
			id: JSONRPCRequest["id"] | undefined,
			result: Record<string, unknown>,
		) => {
			if (id === undefined) {
				throw new Error("cannot respond without a request id");
			}
			return viewTransport.send({ jsonrpc: "2.0", id, result });
		},
		request: async (method: string, params: Record<string, unknown> = {}) => {
			const id = nextId++;
			await viewTransport.send({ jsonrpc: "2.0", id, method, params });
			const response = await waitForResponse(id);
			if (!response || !("result" in response)) {
				throw new Error(`no result for ${method}: ${JSON.stringify(response)}`);
			}
			return response.result;
		},
		requestExpectingError: async (
			method: string,
			params: Record<string, unknown> = {},
		) => {
			const id = nextId++;
			await viewTransport.send({ jsonrpc: "2.0", id, method, params });
			const response = await waitForResponse(id);
			if (!response || !("error" in response)) {
				throw new Error(`no error for ${method}: ${JSON.stringify(response)}`);
			}
			return response.error;
		},
		notificationsFor: (method: string) =>
			received.filter(
				(message) => "method" in message && message.method === method,
			),
	};
};

const view = {
	html: "<!doctype html><p>board</p>",
	csp: { connectDomains: ["https://api.example.com"] },
	permissions: { clipboardWrite: {} },
};

const completeCall: BoundCall = {
	args: { title: "Write tests" },
	argsComplete: true,
	result: { content: [{ type: "text", text: "added" }] },
	cancelled: false,
};

const renderBridge = (
	initial: Partial<UseMcpAppBridgeOptions> & {
		transport: UseMcpAppBridgeOptions["transport"];
	},
) => {
	const queryClient = createTestQueryClient();
	const Wrapper: FC<PropsWithChildren> = ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const store = createChatStore();
	const onAppMessage = vi.fn<(text: string) => Promise<void>>();
	const onOpenLink = vi.fn<(url: string) => Promise<void>>();
	const onClosed = vi.fn();
	const defaults: UseMcpAppBridgeOptions = {
		chatId: CHAT_ID,
		mcpServerConfigId: SERVER_ID,
		resourceUri: RESOURCE_URI,
		appId: APP_ID,
		store,
		hostVersion: "test",
		theme: "dark",
		containerSize: { width: 400, height: 300 },
		view,
		toolCallId: "call-1",
		boundCall: completeCall,
		sandboxOrigin: "https://mcpapp-abc.apps.example.com",
		onAppMessage,
		onOpenLink,
		closing: false,
		onClosed,
		...initial,
	};
	const hook = renderHook(
		(props: UseMcpAppBridgeOptions) => useMcpAppBridge(props),
		{ initialProps: defaults, wrapper: Wrapper },
	);
	return {
		...hook,
		store,
		onAppMessage,
		onOpenLink,
		onClosed,
		update: (next: Partial<UseMcpAppBridgeOptions>) =>
			hook.rerender({ ...defaults, ...next }),
	};
};

const bootView = async (fake: Awaited<ReturnType<typeof createFakeView>>) => {
	await fake.notify("ui/notifications/sandbox-proxy-ready");
	await waitFor(() => {
		expect(
			fake.notificationsFor("ui/notifications/sandbox-resource-ready"),
		).toHaveLength(1);
	});
	await fake.request("ui/initialize", {
		appInfo: { name: "taskboard", version: "0.0.1" },
		appCapabilities: {},
		protocolVersion: "2026-01-26",
	});
	await fake.notify("ui/notifications/initialized");
};

describe("useMcpAppBridge", () => {
	it("hands the view html, csp and permissions to the sandbox proxy", async () => {
		const fake = await createFakeView();
		renderBridge({ transport: fake.hostTransport });

		await fake.notify("ui/notifications/sandbox-proxy-ready");

		await waitFor(() => {
			expect(
				fake.notificationsFor("ui/notifications/sandbox-resource-ready"),
			).toHaveLength(1);
		});
		const [ready] = fake.notificationsFor(
			"ui/notifications/sandbox-resource-ready",
		);
		expect("params" in ready && ready.params).toEqual({
			html: view.html,
			sandbox: "allow-scripts allow-same-origin",
			csp: view.csp,
			permissions: view.permissions,
		});
	});

	it("flushes tool input before the tool result once the view initialized", async () => {
		const fake = await createFakeView();
		const { result } = renderBridge({ transport: fake.hostTransport });

		await bootView(fake);

		await waitFor(() => {
			expect(result.current.phase).toBe("initialized");
			expect(
				fake.notificationsFor("ui/notifications/tool-result"),
			).toHaveLength(1);
		});
		const methods = fake.received
			.filter((message) => "method" in message)
			.map((message) => ("method" in message ? message.method : ""));
		const inputIndex = methods.indexOf("ui/notifications/tool-input");
		const resultIndex = methods.indexOf("ui/notifications/tool-result");
		expect(inputIndex).toBeGreaterThan(-1);
		expect(resultIndex).toBeGreaterThan(inputIndex);
		expect(methods).not.toContain("ui/notifications/tool-input-partial");
		const [toolInput] = fake.notificationsFor("ui/notifications/tool-input");
		expect("params" in toolInput && toolInput.params).toEqual({
			arguments: completeCall.args,
		});
	});

	it("streams partial args and sends the result when it arrives", async () => {
		const fake = await createFakeView();
		const streaming: BoundCall = {
			args: { tit: "" },
			argsComplete: false,
			cancelled: false,
		};
		const { update } = renderBridge({
			transport: fake.hostTransport,
			boundCall: streaming,
		});
		await bootView(fake);

		await waitFor(() => {
			expect(
				fake.notificationsFor("ui/notifications/tool-input-partial"),
			).toHaveLength(1);
		});
		act(() => {
			update({
				boundCall: { ...streaming, args: { title: "Wr" } },
			});
		});
		await waitFor(() => {
			expect(
				fake.notificationsFor("ui/notifications/tool-input-partial"),
			).toHaveLength(2);
		});
		act(() => {
			update({ boundCall: completeCall });
		});
		await waitFor(() => {
			expect(fake.notificationsFor("ui/notifications/tool-input")).toHaveLength(
				1,
			);
			expect(
				fake.notificationsFor("ui/notifications/tool-result"),
			).toHaveLength(1);
		});
	});

	it("sends tool-cancelled when the chat stops without a result", async () => {
		const fake = await createFakeView();
		renderBridge({
			transport: fake.hostTransport,
			boundCall: { args: {}, argsComplete: true, cancelled: true },
		});
		await bootView(fake);

		await waitFor(() => {
			expect(
				fake.notificationsFor("ui/notifications/tool-cancelled"),
			).toHaveLength(1);
		});
		expect(fake.notificationsFor("ui/notifications/tool-result")).toHaveLength(
			0,
		);
	});

	it("proxies tools/call to the chat-scoped endpoint for the app's server", async () => {
		let body: unknown;
		const toolResult = {
			content: [{ type: "text", text: "done" }],
			structuredContent: { tasks: [] },
		};
		server.use(
			http.post(
				`/api/experimental/chats/${CHAT_ID}/mcp-servers/${SERVER_ID}/tools/call`,
				async ({ request }) => {
					body = await request.json();
					return HttpResponse.json({ result: toolResult });
				},
			),
		);
		const fake = await createFakeView();
		renderBridge({ transport: fake.hostTransport });
		await bootView(fake);

		const result = await fake.request("tools/call", {
			name: "add_task",
			arguments: { title: "Ship it" },
		});

		expect(body).toEqual({ name: "add_task", arguments: { title: "Ship it" } });
		expect(result).toEqual(toolResult);
	});

	it("stores ui/update-model-context under the app id", async () => {
		const fake = await createFakeView();
		const { store } = renderBridge({ transport: fake.hostTransport });
		await bootView(fake);

		await fake.request("ui/update-model-context", {
			content: [{ type: "text", text: "Board has 2 tasks" }],
			structuredContent: { count: 2 },
		});

		expect(store.getSnapshot().mcpAppContexts.get(APP_ID)).toEqual({
			mcpServerConfigId: SERVER_ID,
			resourceUri: RESOURCE_URI,
			text: 'Board has 2 tasks\n{"count":2}',
		});
	});

	it("answers ui/message once the confirmation callback resolves", async () => {
		const fake = await createFakeView();
		const { onAppMessage } = renderBridge({ transport: fake.hostTransport });
		onAppMessage.mockResolvedValue(undefined);
		await bootView(fake);

		const result = await fake.request("ui/message", {
			role: "user",
			content: [{ type: "text", text: "Please add a task" }],
		});

		expect(onAppMessage).toHaveBeenCalledWith("Please add a task");
		expect(result).toEqual({});
	});

	it("returns a JSON-RPC error when the message is declined or cannot be sent", async () => {
		const fake = await createFakeView();
		const { onAppMessage } = renderBridge({ transport: fake.hostTransport });
		onAppMessage.mockRejectedValue(
			new Error("Another message is still being sent."),
		);
		await bootView(fake);

		const error = await fake.requestExpectingError("ui/message", {
			role: "user",
			content: [{ type: "text", text: "Please add a task" }],
		});

		expect(error.message).toContain("Another message is still being sent.");
	});

	it("requests teardown before closing an initialized view", async () => {
		const fake = await createFakeView();
		const { unmount } = renderBridge({ transport: fake.hostTransport });
		await bootView(fake);
		await waitFor(() => {
			expect(
				fake.notificationsFor("ui/notifications/tool-result"),
			).toHaveLength(1);
		});

		unmount();

		await waitFor(() => {
			expect(
				fake.received.some(
					(message) =>
						isRequest(message) && message.method === "ui/resource-teardown",
				),
			).toBe(true);
		});
	});

	it("rejects ui/message when the chat cannot accept app messages", async () => {
		const fake = await createFakeView();
		renderBridge({ transport: fake.hostTransport, onAppMessage: undefined });
		await bootView(fake);

		const error = await fake.requestExpectingError("ui/message", {
			role: "user",
			content: [{ type: "text", text: "Please add a task" }],
		});

		expect(error.message).toContain("does not accept messages");
	});

	it("opens http(s) links only after the consent callback resolves", async () => {
		const fake = await createFakeView();
		const { onOpenLink } = renderBridge({ transport: fake.hostTransport });
		onOpenLink.mockResolvedValueOnce(undefined);
		onOpenLink.mockRejectedValueOnce(
			new Error("The user declined the request"),
		);
		await bootView(fake);

		await fake.request("ui/open-link", { url: "https://coder.com/docs" });
		const declined = await fake.requestExpectingError("ui/open-link", {
			url: "https://coder.com/pricing",
		});
		const invalid = await fake.requestExpectingError("ui/open-link", {
			url: "file:///etc/passwd",
		});

		expect(onOpenLink).toHaveBeenCalledTimes(2);
		expect(onOpenLink).toHaveBeenNthCalledWith(1, "https://coder.com/docs");
		expect(declined.message).toContain("declined");
		expect(invalid.message).toContain("http(s)");
	});

	it("tears the view down and reports back when closing", async () => {
		const fake = await createFakeView();
		const { update, onClosed } = renderBridge({
			transport: fake.hostTransport,
		});
		await bootView(fake);
		await waitFor(() => {
			expect(
				fake.notificationsFor("ui/notifications/tool-result"),
			).toHaveLength(1);
		});

		act(() => {
			update({ closing: true });
		});

		const teardown = await waitFor(() => {
			const request = fake.received.find(
				(message): message is JSONRPCRequest =>
					isRequest(message) && message.method === "ui/resource-teardown",
			);
			expect(request).toBeDefined();
			return request;
		});
		expect(onClosed).not.toHaveBeenCalled();
		await fake.respond(teardown?.id, {});
		await waitFor(() => {
			expect(onClosed).toHaveBeenCalledTimes(1);
		});
	});

	it("reports the close immediately when no view ever initialized", async () => {
		const fake = await createFakeView();
		const { update, onClosed } = renderBridge({
			transport: fake.hostTransport,
		});

		act(() => {
			update({ closing: true });
		});

		await waitFor(() => {
			expect(onClosed).toHaveBeenCalledTimes(1);
		});
		expect(
			fake.received.some(
				(message) =>
					isRequest(message) && message.method === "ui/resource-teardown",
			),
		).toBe(false);
	});

	it("fails the boot when the view never initializes", async () => {
		vi.useFakeTimers();
		try {
			const fake = await createFakeView();
			const { result } = renderBridge({ transport: fake.hostTransport });

			await act(async () => {
				await vi.advanceTimersByTimeAsync(15_000);
			});

			expect(result.current.phase).toBe("error");
			expect(result.current.error).toContain(
				"https://mcpapp-abc.apps.example.com",
			);
		} finally {
			vi.useRealTimers();
		}
	});

	it("notifies the view when the theme changes after initialization", async () => {
		const fake = await createFakeView();
		const { update } = renderBridge({ transport: fake.hostTransport });
		await bootView(fake);
		await waitFor(() => {
			expect(
				fake.notificationsFor("ui/notifications/tool-result"),
			).toHaveLength(1);
		});

		act(() => {
			update({ theme: "light" });
		});

		await waitFor(() => {
			const changes = fake.notificationsFor(
				"ui/notifications/host-context-changed",
			);
			expect(
				changes.some(
					(message) =>
						"params" in message &&
						typeof message.params === "object" &&
						message.params !== null &&
						"theme" in message.params &&
						message.params.theme === "light",
				),
			).toBe(true);
		});
	});
});
