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

/** The longest text that a view can put in the composer. */
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
	/** Puts text in the chat composer. The user sends it. */
	insertComposerText: (text: string) => void;
};

export type McpAppBridgeHandle = {
	/** Sends a theme change to the view. */
	setTheme: (theme: McpUiTheme) => void;
	close: () => void;
};

/**
 * Connects an MCP Apps host (AppBridge, SEP-1865) to the view in frame.
 * The browser has no MCP client: coderd talks to the MCP server. The
 * bridge sends the tool input and result only after the view reports
 * that it initialized. AppBridge does not check tool visibility, so
 * coderd refuses view calls to tools that exclude "app". Returns a
 * cleanup that closes the bridge.
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

/** Opens only https links, in a new tab with no opener. */
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

/**
 * Puts the text blocks of a ui/message request in the composer. The
 * spec sends the message to the chat. This prototype lets the user
 * read and send it.
 */
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
