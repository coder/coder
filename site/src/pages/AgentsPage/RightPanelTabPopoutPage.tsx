import { useEffect, useState } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { chat } from "#/api/queries/chats";
import { workspaceById } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { useProxy } from "#/contexts/ProxyContext";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { findWorkspaceAgent } from "#/utils/workspace";
import { useWorkspaceWatch } from "./components/ChatConversation/useWorkspaceWatch";
import { PortPreviewPanel } from "./components/RightPanel/PortPreviewPanel";
import {
	ComposerContext,
	type ComposerHandle,
} from "./context/ComposerContext";
import { useTabPopoutWindow } from "./hooks/useTabPopoutWindow";
import { getPersistedRightPanelTabs } from "./utils/rightPanelTabStorage";
import type { UserRightPanelTab } from "./utils/rightPanelTabs";

/**
 * A right-panel tab in a window of its own, opened from the chat page.
 * The tab is looked up in the same persisted tab list the chat page
 * keeps, so the URL carries nothing but its id, and the window follows
 * the chat page's changes to it. Only port previews can be shown here so
 * far; the page renders the same panel the chat does, with the chat's
 * composer reached over the tab's BroadcastChannel.
 */
export default function RightPanelTabPopoutPage() {
	const { agentId: chatId, tabId } = useParams() as {
		agentId: string;
		tabId: string;
	};
	const tab = usePersistedRightPanelTab(chatId, tabId);

	// The window renders outside the chat page, so it resolves the chat's
	// workspace itself and keeps it live through the same watch.
	const chatQuery = useQuery(chat(chatId));
	const workspaceId = chatQuery.data?.workspace_id;
	const chatAgentId = chatQuery.data?.agent_id;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	useWorkspaceWatch({ workspaceId, agentId: chatId, chatAgentId });
	const workspace = workspaceQuery.data;
	const { proxy } = useProxy();
	const { experiments } = useDashboard();
	const popout = useTabPopoutWindow(tabId);

	return (
		<>
			<title>{pageTitle(tab?.label ?? "Preview", "Agents")}</title>
			<div className="h-screen w-screen bg-surface-primary">
				{chatQuery.error || workspaceQuery.error ? (
					<Centered>
						<ErrorAlert error={chatQuery.error ?? workspaceQuery.error} />
					</Centered>
				) : !tab ? (
					<Message>This tab is no longer open in the chat.</Message>
				) : tab.kind !== "port" ? (
					<Message>This tab cannot be shown in a separate window.</Message>
				) : chatQuery.isSuccess && !workspaceId ? (
					<Message>This chat has no workspace.</Message>
				) : !workspace ? (
					<Centered>
						<Spinner loading className="size-6" />
					</Centered>
				) : (
					<PortPreviewTab
						chatId={chatId}
						tab={tab}
						workspace={workspace}
						host={proxy.preferredWildcardHostname}
						canAnnotate={experiments.includes("chat-ui-annotations")}
						isAgentWorking={popout.chatState.isAgentWorking}
						send={popout.send}
					/>
				)}
			</div>
		</>
	);
}

const PortPreviewTab: React.FC<{
	chatId: string;
	tab: Extract<UserRightPanelTab, { kind: "port" }>;
	workspace: Workspace;
	host: string;
	canAnnotate: boolean;
	isAgentWorking: boolean;
	send: ComposerHandle["send"];
}> = ({ chatId, tab, workspace, host, canAnnotate, isAgentWorking, send }) => {
	const agent = findWorkspaceAgent(workspace, tab.agentId);
	if (!agent) {
		return <Message>This port preview tab is no longer available.</Message>;
	}
	return (
		<ComposerContext value={{ send }}>
			<PortPreviewPanel
				chatId={chatId}
				workspace={workspace}
				agent={agent}
				host={host}
				tab={tab}
				canAnnotate={canAnnotate}
				isAgentWorking={isAgentWorking}
				isPopoutWindow
			/>
		</ComposerContext>
	);
};

const Centered: React.FC<{ children: React.ReactNode }> = ({ children }) => (
	<div className="flex h-full w-full items-center justify-center p-6">
		{children}
	</div>
);

const Message: React.FC<{ children: React.ReactNode }> = ({ children }) => (
	<Centered>
		<span className="text-center text-sm text-content-secondary">
			{children}
		</span>
	</Centered>
);

// The chat page owns the tab list and writes it to localStorage; the
// storage event tells this window when it changes.
function usePersistedRightPanelTab(
	chatId: string,
	tabId: string,
): UserRightPanelTab | undefined {
	const [tab, setTab] = useState(() => findPersistedTab(chatId, tabId));
	useEffect(() => {
		const update = () => setTab(findPersistedTab(chatId, tabId));
		update();
		window.addEventListener("storage", update);
		return () => window.removeEventListener("storage", update);
	}, [chatId, tabId]);
	return tab;
}

function findPersistedTab(
	chatId: string,
	tabId: string,
): UserRightPanelTab | undefined {
	return getPersistedRightPanelTabs(chatId).find((tab) => tab.id === tabId);
}
