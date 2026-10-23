import type { CallToolResult } from "@modelcontextprotocol/client";
import { CallToolResultSchema } from "@modelcontextprotocol/core";
import {
	AppBridge,
	type McpUiMessageRequest,
	type McpUiMessageResult,
	type McpUiOpenLinkRequest,
	type McpUiOpenLinkResult,
	type McpUiTheme,
	PostMessageTransport,
} from "@modelcontextprotocol/ext-apps/app-bridge";
import type { ChatMCPApp } from "#/api/typesGenerated";

// A view must not be able to flood the composer.
const maxComposerTextLength = 16_000;

type McpAppBridgeOptions = {
	app: ChatMCPApp;
	theme: McpUiTheme;
	hostVersion: string;
	/** Runs a tools/call request of the view through coderd. */
	callTool: (
		name: string,
		args: Record<string, unknown> | undefined,
	) => Promise<unknown>;
	/** Inserts the text into the composer. The user still sends it. */
	insertComposerText: (text: string) => void;
};

export type McpAppBridgeHandle = {
	/** Sends only a theme that differs from the last one. */
	setTheme: (theme: McpUiTheme) => void;
	close: () => void;
};

/**
 * AppBridge does not check tool visibility: coderd refuses view calls to
 * tools that exclude "app".
 */
export function attachMcpAppBridge(
	frame: HTMLIFrameElement,
	opts: McpAppBridgeOptions,
): McpAppBridgeHandle {
	const view = frame.contentWindow;
	if (!view) {
		throw new Error("The MCP app iframe has no window.");
	}
	const bridge = new AppBridge(
		null,
		{ name: "coder", version: opts.hostVersion },
		{ openLinks: {}, serverTools: {}, message: { text: {} } },
		{
			hostContext: {
				theme: opts.theme,
				platform: "web",
				displayMode: "inline",
				availableDisplayModes: ["inline"],
			},
		},
	);
	bridge.oninitialized = () => {
		void bridge.sendToolInput({ arguments: opts.app.tool_input });
		const result = parseCallToolResult(opts.app.tool_result);
		if (result) {
			void bridge.sendToolResult(result);
		}
	};
	bridge.oncalltool = async (params) => {
		const raw = await opts.callTool(params.name, params.arguments);
		const result = parseCallToolResult(raw);
		if (!result) {
			throw new Error("The MCP server returned an invalid tool result.");
		}
		return result;
	};
	bridge.onopenlink = async (params) => openLink(params);
	bridge.onmessage = async (params) =>
		insertMessage(params, opts.insertComposerText);

	// The transport only accepts messages from this view's window.
	void bridge.connect(new PostMessageTransport(view, view));
	let theme = opts.theme;
	return {
		setTheme: (next) => {
			if (next === theme) {
				return;
			}
			theme = next;
			void bridge.sendHostContextChange({ theme: next });
		},
		close: () => {
			void bridge.close();
		},
	};
}

/** Validates a raw MCP CallToolResult from coderd. */
function parseCallToolResult(value: unknown): CallToolResult | null {
	const parsed = CallToolResultSchema.safeParse(value);
	return parsed.success ? parsed.data : null;
}

function openLink({
	url,
}: McpUiOpenLinkRequest["params"]): McpUiOpenLinkResult {
	const parsed = URL.parse(url);
	if (parsed?.protocol !== "https:") {
		return { isError: true };
	}
	window.open(parsed.href, "_blank", "noopener,noreferrer");
	return {};
}

// Unlike the spec, this prototype lets the user read and send the message.
function insertMessage(
	params: McpUiMessageRequest["params"],
	insert: (text: string) => void,
): McpUiMessageResult {
	const text = params.content
		.flatMap((block) => (block.type === "text" ? [block.text] : []))
		.join("\n");
	if (!text) {
		return { isError: true };
	}
	insert(text.slice(0, maxComposerTextLength));
	return {};
}
