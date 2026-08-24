import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import {
	canShowPortForwarding,
	usePortsData,
} from "#/modules/resources/usePortsData";
import { portForwardURL } from "#/utils/portForward";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import type { ChatSummaryPreviewLink, ChatSummaryProps } from "./ChatSummary";
import { ChatSummary } from "./ChatSummary";

/**
 * Ports whose purpose is recognizable from the port number alone. Everything
 * else falls back to a generic "Preview" label.
 */
const KNOWN_PORT_LABELS: ReadonlyMap<number, string> = new Map([
	[6006, "Storybook"],
]);

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so the chat and cost queries don't run while the tab is hidden. */
	isVisible: boolean;
	workspace?: Workspace;
	workspaceAgent?: WorkspaceAgent;
	/** Wildcard proxy hostname used to build preview links; empty when unconfigured. */
	wildcardHostname?: string;
};

export const ChatSummaryPanel: FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
	workspace,
	workspaceAgent,
	wildcardHostname = "",
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });

	const chatData = chatQuery.data;
	const rootChatId = getChatCostTreeID(chatData) ?? chatId;
	const costQuery = useQuery({
		...chatCost(rootChatId),
		enabled: isVisible && showCost && chatData !== undefined,
	});

	let content: ReactNode = null;
	if (chatQuery.isError) {
		content = <ErrorAlert error={chatQuery.error} />;
	} else if (chatData) {
		const summaryProps: ChatSummaryProps = {
			summary: chatData.summary,
			isSubagent: Boolean(chatData.parent_chat_id),
			createdAt: chatData.created_at,
			updatedAt: chatData.updated_at,
			costMicros: costQuery.data?.total_cost_micros,
			unpricedRequestCount: costQuery.data?.unpriced_request_count,
			showCost,
			isCostLoading: costQuery.isLoading,
			costError: costQuery.isError,
		};
		content =
			workspace &&
			workspaceAgent &&
			canShowPortForwarding(workspaceAgent, wildcardHostname) ? (
				<ChatSummaryWithPreviews
					workspace={workspace}
					agent={workspaceAgent}
					host={wildcardHostname}
					isVisible={isVisible}
					{...summaryProps}
				/>
			) : (
				<ChatSummary {...summaryProps} />
			);
	}

	return (
		<div className="flex h-full min-h-0 flex-col overflow-y-auto p-4">
			{content}
		</div>
	);
};

/**
 * Wraps ChatSummary with live preview links built from the agent's listening
 * ports, using the same ports queries and URL scheme as the workspace pill.
 */
const ChatSummaryWithPreviews: FC<
	ChatSummaryProps & {
		workspace: Workspace;
		agent: WorkspaceAgent;
		host: string;
		isVisible: boolean;
	}
> = ({ workspace, agent, host, isVisible, ...summaryProps }) => {
	const portsData = usePortsData(
		workspace,
		agent,
		isVisible && agent.status === "connected",
	);

	const previews: ChatSummaryPreviewLink[] = (portsData.listeningPorts ?? [])
		.toSorted((a, b) => a.port - b.port)
		.map((port) => ({
			label: KNOWN_PORT_LABELS.get(port.port) ?? "Preview",
			port: port.port,
			url: portForwardURL(
				host,
				port.port,
				agent.name,
				workspace.name,
				workspace.owner_name,
				portsData.protocol,
			),
		}));

	return <ChatSummary {...summaryProps} previews={previews} />;
};
