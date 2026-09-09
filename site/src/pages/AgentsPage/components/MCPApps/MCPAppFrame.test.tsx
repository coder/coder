import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockChatMCPApp, MockChatMessage } from "#/testHelpers/chatEntities";
import { MockBuildInfo } from "#/testHelpers/entities";
import themes from "#/theme";
import { ThemeContextProvider } from "#/theme/context";
import { createChatStore } from "../ChatConversation/chatStore";
import {
	mergeTools,
	parseMessageContent,
} from "../ChatConversation/messageParsing";
import { Tool } from "../ChatElements/tools/Tool";
import * as bridge from "./bridge";
import { MCPAppContext } from "./MCPAppContext";
import { MCPAppFrame } from "./MCPAppFrame";
import { MCPAppPanel } from "./MCPAppPanel";

const dashboard = vi.hoisted(() => ({
	experiments: ["chat-mcp-apps"],
}));
vi.mock("@pierre/diffs/react", () => ({
	File: () => null,
	MultiFileDiff: () => null,
	PatchDiff: () => null,
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ ...dashboard, buildInfo: MockBuildInfo }),
}));

const send = (
	source: Window | null,
	method: string,
	id?: number,
	params?: unknown,
) => {
	act(() => {
		window.dispatchEvent(
			new MessageEvent("message", {
				source,
				data: { jsonrpc: "2.0", method, id, params },
			}),
		);
	});
};
const themeWrapper = ({ children }: { children: React.ReactNode }) => (
	<ThemeContextProvider theme={themes.dark}>{children}</ThemeContextProvider>
);

let intersect = () => {};
class MockIntersectionObserver {
	observe = vi.fn();
	disconnect = vi.fn();
	unobserve = vi.fn();
	constructor(callback: (entries: Array<{ isIntersecting: boolean }>) => void) {
		intersect = () => callback([{ isIntersecting: true }]);
	}
}

beforeEach(() => {
	vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
});

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
	dashboard.experiments = ["chat-mcp-apps"];
});

it("keeps the bridge connected with current inputs across rerenders and disconnects on unmount", () => {
	vi.useFakeTimers();
	const props = {
		src: "about:blank",
		title: "Sales",
		args: { metric: "sales" },
		result: MockChatMCPApp.result,
		displayMode: "inline" as const,
		fallback: null,
	};
	const renderFrame = (theme: typeof themes.dark, args = props.args) => (
		<ThemeContextProvider theme={theme}>
			<MCPAppFrame {...props} args={args} />
		</ThemeContextProvider>
	);
	const view = render(renderFrame(themes.dark));
	const frame = screen.getByTitle<HTMLIFrameElement>("Sales");
	expect(frame.getAttribute("sandbox")).toBe("allow-scripts");
	const source = frame.contentWindow;
	if (!source) throw new Error("No iframe window");
	const post = vi.spyOn(source, "postMessage");
	send(source, "ui/initialize", 1);
	view.rerender(renderFrame(themes.dark, { metric: "new sales" }));
	send(source, "ui/notifications/initialized");
	expect(post).toHaveBeenCalledWith(
		{
			jsonrpc: "2.0",
			method: "ui/notifications/tool-input",
			params: { arguments: { metric: "new sales" } },
		},
		"*",
	);
	view.rerender(renderFrame(themes.light, { metric: "new sales" }));
	expect(post).toHaveBeenLastCalledWith(
		{
			jsonrpc: "2.0",
			method: "ui/notifications/host-context-changed",
			params: { theme: "light" },
		},
		"*",
	);
	send(source, "ui/notifications/size-change", undefined, {
		height: 999999,
		width: -1,
	});
	act(() => vi.advanceTimersToNextFrame());
	send(source, "ui/notifications/size-change", undefined, { height: 240 });
	send(source, "ui/notifications/size-change", undefined, { width: 640 });
	act(() => vi.advanceTimersToNextFrame());
	view.unmount();
	post.mockClear();
	send(source, "ui/initialize", 2);
	expect(post).not.toHaveBeenCalled();
});

it.each(["timeout", "error"])(
	"offers working fallback output after initialization %s",
	async (failure) => {
		vi.useFakeTimers();
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		const onRawOutput = vi.fn();
		render(
			<MCPAppFrame
				src="about:blank"
				title="Sales"
				args={{}}
				result={MockChatMCPApp.result}
				displayMode="inline"
				fallback={<button onClick={onRawOutput}>Raw output</button>}
			/>,
			{ wrapper: themeWrapper },
		);
		if (failure === "timeout") act(() => vi.advanceTimersByTime(15000));
		else fireEvent.error(screen.getByTitle("Sales"));
		await user.click(screen.getByRole("button", { name: "Raw output" }));
		expect(onRawOutput).toHaveBeenCalledOnce();
	},
);

it("stops trusting a frame that navigates after the served document", async () => {
	const onRawOutput = vi.fn();
	render(
		<MCPAppFrame
			src="about:blank"
			title="Sales"
			args={{}}
			result={MockChatMCPApp.result}
			displayMode="inline"
			fallback={<button onClick={onRawOutput}>Raw output</button>}
		/>,
		{ wrapper: themeWrapper },
	);
	const frame = screen.getByTitle<HTMLIFrameElement>("Sales");
	const source = frame.contentWindow;
	if (!source) throw new Error("No iframe window");
	const post = vi.spyOn(source, "postMessage");
	send(source, "ui/initialize", 1);
	send(source, "ui/notifications/initialized");
	post.mockClear();
	fireEvent.load(frame);
	fireEvent.load(frame);
	expect(screen.queryByTitle("Sales")).toBeNull();
	send(source, "ui/initialize", 2);
	expect(post).not.toHaveBeenCalled();
	await userEvent.click(screen.getByRole("button", { name: "Raw output" }));
	expect(onRawOutput).toHaveBeenCalledOnce();
});

it("waits for the call before initializing inline and opening the result in the panel", async () => {
	const onOpenApp = vi.fn();
	const connect = vi.spyOn(bridge, "connectMCPApp");
	const parsed = parseMessageContent([
		{
			type: "tool-call",
			tool_call_id: "tool-1",
			tool_name: "Sales",
			args: { metric: "sales" },
		},
		{
			type: "tool-result",
			tool_call_id: "tool-1",
			tool_name: "Sales",
			result: { output: "Sales chart" },
			mcp_app: MockChatMCPApp,
		},
	]);
	const orphan = mergeTools([], parsed.toolResults)[0];
	const paired = mergeTools(parsed.toolCalls, parsed.toolResults)[0];
	if (!orphan || !paired) throw new Error("Missing tool result");
	const renderTool = (tool: typeof orphan) => (
		<MCPAppContext value={{ chatId: "chat", onOpenApp }}>
			<Tool {...tool} toolCallId={tool.id} />
		</MCPAppContext>
	);
	const view = render(renderTool(orphan), { wrapper: themeWrapper });
	expect(connect).not.toHaveBeenCalled();
	view.rerender(renderTool(paired));
	expect(screen.queryByTitle("Sales")).toBeNull();
	act(intersect);
	const source = screen.getByTitle<HTMLIFrameElement>("Sales").contentWindow;
	if (!source) throw new Error("No iframe window");
	const post = vi.spyOn(source, "postMessage");
	send(source, "ui/initialize", 1);
	send(source, "ui/notifications/initialized");
	expect(post).toHaveBeenCalledWith(
		{
			jsonrpc: "2.0",
			method: "ui/notifications/tool-input",
			params: { arguments: { metric: "sales" } },
		},
		"*",
	);
	await userEvent.click(screen.getByRole("button", { name: "Open in panel" }));
	expect(onOpenApp).toHaveBeenCalledWith({
		id: "mcp-app-tool-1",
		kind: "mcp_app",
		toolCallId: "tool-1",
		label: "Sales",
		serverName: "charts",
		resourceUri: "ui://charts/sales",
	});
});

it("keeps ordinary tool output usable while the experiment is disabled", async () => {
	dashboard.experiments = [];
	const onOpenApp = vi.fn();
	render(
		<MCPAppContext value={{ chatId: "chat", onOpenApp }}>
			<Tool
				name="Sales"
				toolCallId="tool-1"
				mcpApp={MockChatMCPApp}
				args={{}}
				result="Sales chart"
			/>
		</MCPAppContext>,
		{ wrapper: themeWrapper },
	);
	const toggle = screen.getByRole("button");
	await userEvent.click(toggle);
	expect(onOpenApp).not.toHaveBeenCalled();
});

it("waits for the original call before initializing a restored panel", () => {
	const tab = {
		id: "mcp-app-tool-1",
		kind: "mcp_app" as const,
		toolCallId: "tool-1",
		label: "Sales",
		serverName: "charts",
		resourceUri: "ui://charts/sales",
	};
	const store = createChatStore();
	store.replaceMessages([
		{
			...MockChatMessage,
			content: [
				{ type: "tool-call", tool_call_id: "other", args: { metric: "wrong" } },
				{
					type: "tool-result",
					tool_call_id: "tool-1",
					mcp_app: MockChatMCPApp,
				},
			],
		},
	]);
	const connect = vi.spyOn(bridge, "connectMCPApp");
	render(<MCPAppPanel chatId="chat" tab={tab} store={store} />, {
		wrapper: themeWrapper,
	});
	expect(connect).not.toHaveBeenCalled();
	act(() => {
		store.upsertDurableMessage({
			...MockChatMessage,
			id: MockChatMessage.id + 1,
			content: [
				{
					type: "tool-call",
					tool_call_id: "tool-1",
					args: { metric: "sales" },
				},
			],
		});
	});
	expect(connect).toHaveBeenCalledOnce();

	const source = screen.getByTitle<HTMLIFrameElement>("Sales").contentWindow;
	if (!source) throw new Error("No iframe window");
	const post = vi.spyOn(source, "postMessage");
	send(source, "ui/initialize", 1);
	expect(post).toHaveBeenCalledWith(
		expect.objectContaining({
			result: expect.objectContaining({
				hostContext: expect.objectContaining({ displayMode: "fullscreen" }),
			}),
		}),
		"*",
	);
	send(source, "ui/notifications/initialized");
	expect(post).toHaveBeenCalledWith(
		{
			jsonrpc: "2.0",
			method: "ui/notifications/tool-input",
			params: { arguments: { metric: "sales" } },
		},
		"*",
	);
	expect(post).toHaveBeenCalledWith(
		{
			jsonrpc: "2.0",
			method: "ui/notifications/tool-result",
			params: {
				content: [{ type: "text", text: "Sales chart" }],
				structuredContent: { sales: [10, 20] },
				isError: false,
			},
		},
		"*",
	);
});
