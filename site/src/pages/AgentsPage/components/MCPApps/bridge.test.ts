import { MockChatMCPApp } from "#/testHelpers/chatEntities";
import { connectMCPApp } from "./bridge";

const context = {
	theme: "dark" as const,
	displayMode: "inline" as const,
	containerDimensions: { width: 600, height: 320 },
	platform: "web" as const,
	userAgent: "test-browser",
};

const setup = (result: unknown = MockChatMCPApp.result) => {
	const frame = document.createElement("iframe");
	document.body.append(frame);
	const source = frame.contentWindow;
	if (!source) throw new Error("No iframe window");
	const post = vi.spyOn(source, "postMessage");
	const onReady = vi.fn();
	const onSizeChange = vi.fn();
	const { disconnect, notifyHostContextChanged } = connectMCPApp({
		frame,
		hostVersion: "v1",
		getContext: () => context,
		getToolData: () => ({ args: { metric: "sales" }, result }),
		onReady,
		onSizeChange,
	});
	const send = (method: string, params?: unknown, id?: number | string) =>
		window.dispatchEvent(
			new MessageEvent("message", {
				source,
				data: { jsonrpc: "2.0", method, params, id },
			}),
		);
	return {
		post,
		send,
		onReady,
		onSizeChange,
		disconnect,
		notifyHostContextChanged,
	};
};

afterEach(() => {
	document.body.replaceChildren();
	vi.restoreAllMocks();
});

it("negotiates the stable protocol and sends input before original results once initialized", () => {
	const result = {
		content: [
			{ type: "text", text: "chart", annotations: { audience: ["user"] } },
			{ type: "image", data: "image", mimeType: "image/png" },
			{ type: "audio", data: "audio", mimeType: "audio/wav" },
			{
				type: "resource",
				resource: {
					uri: "file:///chart",
					mimeType: "text/plain",
					text: "embedded",
					_meta: { custom: true },
				},
			},
			{
				type: "resource",
				resource: {
					uri: "file:///blob",
					mimeType: "image/png",
					blob: "bytes",
				},
			},
			{
				type: "resource_link",
				name: "Sales report",
				uri: "https://example.com/chart",
				mimeType: "text/html",
			},
		],
		structuredContent: { preserve_snake_case: true },
		isError: false,
		_meta: { view: "sales" },
	};
	const bridge = setup(result);
	bridge.send("ui/notifications/initialized");
	expect(bridge.post).not.toHaveBeenCalled();
	bridge.send("ui/initialize", {}, 1);
	expect(bridge.post).toHaveBeenLastCalledWith(
		{
			jsonrpc: "2.0",
			id: 1,
			result: {
				protocolVersion: "2026-01-26",
				hostInfo: { name: "coder", version: "v1" },
				hostCapabilities: { openLinks: {} },
				hostContext: context,
			},
		},
		"*",
	);
	bridge.send("ui/notifications/initialized");
	expect(bridge.post).toHaveBeenNthCalledWith(
		2,
		{
			jsonrpc: "2.0",
			method: "ui/notifications/tool-input",
			params: { arguments: { metric: "sales" } },
		},
		"*",
	);
	expect(bridge.post).toHaveBeenNthCalledWith(
		3,
		{
			jsonrpc: "2.0",
			method: "ui/notifications/tool-result",
			params: result,
		},
		"*",
	);
	bridge.send("ui/notifications/initialized");
	expect(bridge.onReady).toHaveBeenCalledTimes(1);
	expect(bridge.post).toHaveBeenCalledTimes(3);
	bridge.disconnect();
});

it("notifies only initialized apps about host context changes", () => {
	const bridge = setup();
	bridge.send("ui/initialize", {}, 1);
	bridge.notifyHostContextChanged({ theme: "light" });
	expect(bridge.post).toHaveBeenCalledTimes(1);
	bridge.send("ui/notifications/initialized");
	bridge.notifyHostContextChanged({ theme: "light" });
	expect(bridge.post).toHaveBeenLastCalledWith(
		{
			jsonrpc: "2.0",
			method: "ui/notifications/host-context-changed",
			params: { theme: "light" },
		},
		"*",
	);
	bridge.disconnect();
});

it("ignores other windows, malformed messages, and messages after cleanup", () => {
	const bridge = setup();
	for (const source of [window, null])
		window.dispatchEvent(
			new MessageEvent("message", {
				source,
				data: { jsonrpc: "2.0", id: 1, method: "ui/initialize" },
			}),
		);
	window.dispatchEvent(new MessageEvent("message", { data: null }));
	expect(bridge.post).not.toHaveBeenCalled();
	bridge.disconnect();
	bridge.send("ui/initialize", {}, 2);
	expect(bridge.post).not.toHaveBeenCalled();
});

it.each([
	"tools/call",
	"resources/read",
	"ui/message",
	"ui/update-model-context",
	"ui/download-file",
	"unknown",
])("rejects unsupported request %s", (method) => {
	const bridge = setup();
	bridge.send(method, {}, "request");
	expect(bridge.post).toHaveBeenCalledWith(
		{
			jsonrpc: "2.0",
			id: "request",
			error: { code: -32601, message: "Method not found" },
		},
		"*",
	);
	bridge.disconnect();
});

it("opens only absolute HTTP(S) links without opener or referrer access", () => {
	const open = vi.spyOn(window, "open").mockReturnValue(null);
	const bridge = setup();
	for (const url of ["https://example.com/x", "http://example.com/"])
		bridge.send("ui/open-link", { url }, 1);
	expect(open.mock.calls).toEqual([
		["https://example.com/x", "_blank", "noopener,noreferrer"],
		["http://example.com/", "_blank", "noopener,noreferrer"],
	]);
	for (const url of [
		// oxlint-disable-next-line no-script-url
		"javascript:alert(1)",
		"data:text/html,test",
		"file:///tmp/x",
		"/relative",
		"//example.com",
		"not a url",
		undefined,
	]) {
		bridge.send("ui/open-link", { url }, 2);
		expect(bridge.post).toHaveBeenLastCalledWith(
			expect.objectContaining({
				id: 2,
				error: expect.objectContaining({ code: -32602 }),
			}),
			"*",
		);
	}
	expect(open).toHaveBeenCalledTimes(2);
	bridge.disconnect();
});

it("clamps finite sizes and does not change the display mode", () => {
	const bridge = setup();
	bridge.send("ui/initialize", {}, 1);
	bridge.send("ui/notifications/initialized");
	for (const height of [
		-1,
		480,
		999999,
		Number.NaN,
		Number.POSITIVE_INFINITY,
		"500",
	])
		bridge.send("ui/notifications/size-change", { height });
	expect(bridge.onSizeChange.mock.calls).toEqual([
		[{ height: 100 }],
		[{ height: 480 }],
		[{ height: 1200 }],
	]);
	bridge.onSizeChange.mockClear();
	for (const width of [
		-1,
		640,
		999999,
		Number.NaN,
		Number.POSITIVE_INFINITY,
		"500",
	])
		bridge.send("ui/notifications/size-change", { width });
	expect(bridge.onSizeChange.mock.calls).toEqual([
		[{ width: 100 }],
		[{ width: 640 }],
		[{ width: 1200 }],
	]);
	bridge.send("ui/request-display-mode", { mode: "fullscreen" }, 2);
	expect(bridge.post).toHaveBeenLastCalledWith(
		{ jsonrpc: "2.0", id: 2, result: { mode: "inline" } },
		"*",
	);
	bridge.disconnect();
});

it("logs app notifications without putting them in chat", () => {
	const debug = vi.spyOn(console, "debug").mockImplementation(() => {});
	const bridge = setup();
	bridge.send("notifications/message", { level: "info", data: "hello" });
	expect(debug).toHaveBeenCalledWith("MCP App:", {
		level: "info",
		data: "hello",
	});
	expect(bridge.post).not.toHaveBeenCalled();
	bridge.disconnect();
});

it.each([null, [], "invalid"])(
	"sends an empty result for invalid metadata %j",
	(result) => {
		const bridge = setup(result);
		bridge.send("ui/initialize", {}, 1);
		bridge.send("ui/notifications/initialized");
		expect(bridge.post).toHaveBeenLastCalledWith(
			{
				jsonrpc: "2.0",
				method: "ui/notifications/tool-result",
				params: { content: [], isError: false },
			},
			"*",
		);
		bridge.disconnect();
	},
);
