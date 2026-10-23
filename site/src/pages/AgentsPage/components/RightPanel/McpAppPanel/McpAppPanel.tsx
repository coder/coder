import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { callChatMCPAppTool, openChatMCPApp } from "#/api/queries/chats";
import type { ChatMCPApp } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useTheme } from "#/theme/context";
import type { UserRightPanelTab } from "../../../utils/rightPanelTabs";
import {
	type ChatStoreState,
	useChatSelector,
	type useChatStore,
} from "../../ChatConversation/chatStore";
import { attachMcpAppBridge, type McpAppBridgeHandle } from "./mcpAppBridge";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

type McpAppTab = Extract<UserRightPanelTab, { kind: "mcp_app" }>;

/**
 * The view data of a tool call: from the live stream while the turn
 * runs, then from the persisted tool-result part.
 */
function findMcpAppInChat(
	state: ChatStoreState,
	toolCallId: string,
): ChatMCPApp | undefined {
	const live = state.streamState?.toolResults[toolCallId]?.mcpApp;
	if (live) {
		return live;
	}
	for (const message of state.messagesByID.values()) {
		for (const part of message.content ?? []) {
			if (part.type === "tool-result" && part.tool_call_id === toolCallId) {
				return part.mcp_app;
			}
		}
	}
	return undefined;
}

const PanelMessage: React.FC<{ message: string }> = ({ message }) => (
	<div className="flex h-full min-h-0 items-center justify-center px-6 text-center text-xs text-content-secondary">
		{message}
	</div>
);

type McpAppPanelProps = {
	chatId: string;
	tab: McpAppTab;
	store: ChatStoreHandle;
	onInsertComposerText: (text: string) => void;
};

/**
 * Shows an MCP Apps view of an org MCP server in a right panel tab. The
 * view data comes from a tool result in the chat, or from an entry point
 * that coderd opens with a tool call of {}.
 */
export const McpAppPanel: React.FC<McpAppPanelProps> = ({
	chatId,
	tab,
	store,
	onInsertComposerText,
}) => {
	const toolCallId = tab.toolCallId;
	const fromChat = useChatSelector(store, (state) =>
		toolCallId ? findMcpAppInChat(state, toolCallId) : undefined,
	);
	const opened = useQuery({
		...openChatMCPApp(
			chatId,
			tab.mcpServerConfigId,
			tab.entrypointToolName ?? "",
		),
		enabled: tab.entrypointToolName !== undefined,
	});
	const app = fromChat ?? opened.data;

	if (!app) {
		if (opened.isError) {
			return (
				<div className="p-4">
					<ErrorAlert error={opened.error} />
				</div>
			);
		}
		if (tab.entrypointToolName !== undefined) {
			return <Loader label="Opening the MCP app" />;
		}
		return (
			<PanelMessage message="The tool result for this view is not loaded. Scroll the chat to it, or run the tool again." />
		);
	}
	return (
		<McpAppFrame
			// A new source needs a new view: the bridge sends the data once.
			key={`${tab.id}:${app.resource_uri}`}
			chatId={chatId}
			mcpServerConfigId={tab.mcpServerConfigId}
			title={tab.label}
			app={app}
			onInsertComposerText={onInsertComposerText}
		/>
	);
};

type McpAppFrameProps = {
	chatId: string;
	mcpServerConfigId: string;
	title: string;
	app: ChatMCPApp;
	onInsertComposerText: (text: string) => void;
};

const McpAppFrame: React.FC<McpAppFrameProps> = ({
	chatId,
	mcpServerConfigId,
	title,
	app,
	onInsertComposerText,
}) => {
	const frameRef = useRef<HTMLIFrameElement>(null);
	const bridgeRef = useRef<McpAppBridgeHandle | null>(null);
	const loadCount = useRef(0);
	const [leftView, setLeftView] = useState(false);
	const theme = useTheme().palette.mode;
	const { buildInfo } = useDashboard();
	const { mutateAsync: callTool } = useMutation(
		callChatMCPAppTool(chatId, mcpServerConfigId),
	);

	const bridgeOptions = useEffectEvent(() => ({
		app,
		theme,
		hostVersion: buildInfo.version,
		callTool: (name: string, args: Record<string, unknown> | undefined) =>
			callTool({ name, arguments: args }),
		insertComposerText: onInsertComposerText,
	}));

	// The bridge belongs to the iframe's browsing context. It connects
	// before the view loads, and the key of this component replaces the
	// iframe when the view data changes.
	useEffect(() => {
		const frame = frameRef.current;
		if (!frame) {
			return;
		}
		const handle = attachMcpAppBridge(frame, bridgeOptions());
		bridgeRef.current = handle;
		return () => {
			bridgeRef.current = null;
			handle.close();
		};
	}, []);

	useEffect(() => {
		bridgeRef.current?.setTheme(theme);
	}, [theme]);

	if (leftView) {
		return (
			<PanelMessage message="The MCP app left its page, so Coder removed it. Close the tab and open the app again." />
		);
	}

	const src = `/api/experimental/chats/${chatId}/mcp-apps/${mcpServerConfigId}/view?uri=${encodeURIComponent(app.resource_uri)}`;
	return (
		<iframe
			ref={frameRef}
			title={title}
			src={src}
			// The view page also has a CSP sandbox header, so it keeps an
			// opaque origin when someone opens the URL as a top-level page.
			sandbox="allow-scripts allow-forms"
			className="size-full border-0 bg-surface-primary"
			onLoad={() => {
				// A second load is a different document that could talk to
				// the bridge as the view.
				loadCount.current += 1;
				if (loadCount.current > 1) {
					setLeftView(true);
				}
			}}
		/>
	);
};
