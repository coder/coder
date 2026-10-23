import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { callChatMCPAppTool, openChatMCPApp } from "#/api/queries/chats";
import type { ChatMCPApp } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useTheme } from "#/theme/context";
import type { McpAppRightPanelTab } from "../../../utils/mcpAppTabs";
import {
	type ChatStore,
	type ChatStoreState,
	useChatSelector,
} from "../../ChatConversation/chatStore";
import { UnavailableTabMessage } from "../UnavailableTabMessage";
import { mcpAppToolResults } from "./chatMcpApps";
import { attachMcpAppBridge, type McpAppBridgeHandle } from "./mcpAppBridge";

// The live stream wins while a turn runs.
function findMcpAppInChat(
	state: ChatStoreState,
	toolCallId: string,
): ChatMCPApp | undefined {
	const live = state.streamState?.toolResults[toolCallId]?.mcpApp;
	if (live) {
		return live;
	}
	for (const result of mcpAppToolResults(state.messagesByID.values())) {
		if (result.toolCallId === toolCallId) {
			return result.app;
		}
	}
	return undefined;
}

type McpAppPanelProps = {
	chatId: string;
	tab: McpAppRightPanelTab;
	store: ChatStore;
	onInsertComposerText: (text: string) => void;
};

/**
 * A tab with toolCallId shows that tool result. A tab with
 * entrypointToolName asks coderd to call the tool with {} first.
 */
export const McpAppPanel: React.FC<McpAppPanelProps> = ({
	chatId,
	tab,
	store,
	onInsertComposerText,
}) => {
	const frameProps = {
		chatId,
		mcpServerConfigId: tab.mcpServerConfigId,
		title: tab.label,
		onInsertComposerText,
	};
	if (tab.toolCallId !== undefined) {
		return (
			<McpAppToolResultView
				store={store}
				toolCallId={tab.toolCallId}
				frameProps={frameProps}
			/>
		);
	}
	if (tab.entrypointToolName !== undefined) {
		return (
			<McpAppEntrypointView
				toolName={tab.entrypointToolName}
				frameProps={frameProps}
			/>
		);
	}
	return <UnavailableTabMessage message="This MCP app tab has no source." />;
};

type FrameProps = Omit<McpAppFrameProps, "app">;

const McpAppToolResultView: React.FC<{
	store: ChatStore;
	toolCallId: string;
	frameProps: FrameProps;
}> = ({ store, toolCallId, frameProps }) => {
	const app = useChatSelector(store, (state) =>
		findMcpAppInChat(state, toolCallId),
	);
	if (!app) {
		return (
			<UnavailableTabMessage message="The tool result for this view is not loaded. Scroll the chat to it, or run the tool again." />
		);
	}
	// The bridge sends the view data once, so a new view URI needs a new frame.
	return <McpAppFrame key={app.resource_uri} app={app} {...frameProps} />;
};

const McpAppEntrypointView: React.FC<{
	toolName: string;
	frameProps: FrameProps;
}> = ({ toolName, frameProps }) => {
	const opened = useQuery(
		openChatMCPApp(frameProps.chatId, frameProps.mcpServerConfigId, toolName),
	);
	if (opened.isError) {
		return (
			<div className="p-4">
				<ErrorAlert error={opened.error} />
			</div>
		);
	}
	if (!opened.data) {
		return <Loader label="Opening the MCP app" />;
	}
	return (
		<McpAppFrame
			key={opened.data.resource_uri}
			app={opened.data}
			{...frameProps}
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

	// The bridge belongs to the iframe's browsing context, so it connects
	// once, before the view loads.
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
			<UnavailableTabMessage message="The MCP app left its page, so Coder removed it. Close the tab and open the app again." />
		);
	}

	return (
		<iframe
			ref={frameRef}
			title={title}
			src={`/api/experimental/chats/${chatId}/mcp-apps/${mcpServerConfigId}/view?uri=${encodeURIComponent(app.resource_uri)}`}
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
