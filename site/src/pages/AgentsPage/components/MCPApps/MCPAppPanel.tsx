import type {
	ChatToolCallPart,
	ChatToolResultPart,
} from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import {
	type ChatStore,
	selectMessagesByID,
	useChatSelector,
} from "../ChatConversation/chatStore";
import { GenericToolRenderer } from "../ChatElements/tools/Tool";
import { MCPAppFrame } from "./MCPAppFrame";
import { mcpAppResourceURL } from "./resourceURL";

interface MCPAppPanelProps {
	chatId: string;
	tab: Extract<UserRightPanelTab, { kind: "mcp_app" }>;
	store: ChatStore;
}

export const MCPAppPanel = ({ chatId, tab, store }: MCPAppPanelProps) => {
	const { experiments } = useDashboard();
	const messages = useChatSelector(store, selectMessagesByID);
	let call: ChatToolCallPart | undefined;
	let result: ChatToolResultPart | undefined;
	for (const message of messages.values()) {
		for (const part of message.content ?? []) {
			if (
				(part.type === "tool-call" || part.type === "tool-result") &&
				part.tool_call_id === tab.toolCallId
			) {
				if (part.type === "tool-call") call = part;
				else result = part;
			}
		}
	}
	const app = result?.mcp_app;
	if (
		!experiments.includes("chat-mcp-apps") ||
		!call ||
		!app ||
		app.server_name !== tab.serverName ||
		app.resource_uri !== tab.resourceUri
	) {
		return (
			<div className="flex h-full items-center justify-center p-6 text-center text-sm text-content-secondary">
				This app is unavailable. Load its original message to open it again.
			</div>
		);
	}
	const src = mcpAppResourceURL(chatId, app.server_name, app.resource_uri);
	return (
		<MCPAppFrame
			key={src}
			src={src}
			title={tab.label}
			args={call?.args}
			result={app.result}
			displayMode="pip"
			fallback={
				<GenericToolRenderer
					name={result?.tool_name ?? tab.label}
					args={call?.args}
					result={result?.result}
					status="completed"
					isError={result?.is_error ?? false}
				/>
			}
		/>
	);
};
