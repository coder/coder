import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, type ReactNode, useLayoutEffect, useRef } from "react";
import { useQuery } from "react-query";
import { agentLogs, workspaceById } from "#/api/queries/workspaces";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import type { Line } from "#/components/Logs/LogLine";
import { DEFAULT_LOG_LINE_SIDE_PADDING } from "#/components/Logs/Logs";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { AgentLogLine } from "#/modules/resources/AgentLogs/AgentLogLine";
import { useAgentLogs } from "#/modules/resources/useAgentLogs";
import { findWorkspaceAgent } from "#/utils/workspace";
import {
	useChatAgentId,
	useChatBuildId,
	useChatWorkspaceId,
} from "../../../context/ChatWorkspaceContext";
import type { ToolStatus } from "./utils";

interface WorkspaceAgentLogSectionProps {
	status: ToolStatus;
	/** Build ID from the completed tool result. */
	buildId?: string;
}

/** Agent startup logs: streamed while the tool runs, fetched once it completes. */
export const WorkspaceAgentLogSection: FC<WorkspaceAgentLogSectionProps> = ({
	status,
	buildId,
}) => {
	const isRunning = status === "running";
	const workspaceId = useChatWorkspaceId();
	const chatBuildId = useChatBuildId();
	const chatAgentId = useChatAgentId();

	// Updated live by the workspace watch socket; do not poll.
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const workspace = workspaceQuery.data;
	const relevantBuildId = isRunning ? chatBuildId : buildId;
	// After a rebuild, chat.agent_id may still be the previous build's agent.
	const buildIsCurrent =
		workspace !== undefined &&
		relevantBuildId !== undefined &&
		workspace.latest_build.id === relevantBuildId &&
		workspace.latest_build.status === "running";

	if (!buildIsCurrent) {
		return null;
	}

	const agent = chatAgentId
		? findWorkspaceAgent(workspace, chatAgentId)
		: undefined;
	if (!agent) {
		return isRunning ? (
			<Notice
				icon={
					<LoaderIcon className="size-3 animate-spin motion-reduce:animate-none" />
				}
			>
				Waiting for workspace agent…
			</Notice>
		) : null;
	}

	return (
		<AgentStartupLogs key={agent.id} agent={agent} isRunning={isRunning} />
	);
};

interface AgentStartupLogsProps {
	agent: WorkspaceAgent;
	isRunning: boolean;
}

const AgentStartupLogs: FC<AgentStartupLogsProps> = ({ agent, isRunning }) => {
	const streamedLogs = useAgentLogs({ agentId: agent.id, enabled: isRunning });
	const completedLogsQuery = useQuery({
		...agentLogs(agent.id),
		enabled: !isRunning,
		// Cached logs may have been fetched before the agent finished starting.
		staleTime: 0,
		refetchOnMount: true,
	});
	const logs = isRunning ? streamedLogs : completedLogsQuery.data;
	const hasLogs = logs !== undefined && logs.length > 0;

	const endRef = useRef<HTMLDivElement>(null);
	useLayoutEffect(() => {
		if (isRunning && logs && logs.length > 0) {
			endRef.current?.scrollIntoView({ block: "end" });
		}
	}, [isRunning, logs]);

	// isError can be set while data from an earlier fetch is still present.
	if (!isRunning && completedLogsQuery.isError && !hasLogs) {
		return (
			<Notice icon={<TriangleAlertIcon className="size-3" />}>
				Failed to load agent logs.
			</Notice>
		);
	}

	if (!hasLogs) {
		if (!isRunning && completedLogsQuery.isSuccess) {
			return <Notice>No agent logs available.</Notice>;
		}
		return (
			<Notice
				icon={
					<LoaderIcon className="size-3 animate-spin motion-reduce:animate-none" />
				}
			>
				{isRunning ? "Waiting for agent logs…" : "Loading agent logs…"}
			</Notice>
		);
	}

	const lines = logs.map<Line>((log) => ({
		id: log.id,
		time: log.created_at,
		output: log.output,
		level: log.level,
		sourceId: log.source_id,
	}));

	return (
		<ScrollArea
			className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
			viewportClassName="max-h-64"
			viewportTabIndex={0}
			viewportAriaLabel="Workspace agent startup log"
			scrollBarClassName="w-1.5"
		>
			<div className="font-mono">
				<div
					className="flex items-center bg-surface-primary font-sans text-xs font-semibold leading-none"
					style={{
						padding: `12px var(--log-line-side-padding, ${DEFAULT_LOG_LINE_SIDE_PADDING}px)`,
					}}
				>
					<div>Agent startup</div>
					<div className="ml-auto text-xs text-content-secondary">
						{agent.name}
					</div>
				</div>
				<div className="py-2 bg-surface-primary">
					{lines.map((line) => (
						<AgentLogLine key={line.id} line={line} sourceIcon={null} />
					))}
				</div>
				<div ref={endRef} />
			</div>
		</ScrollArea>
	);
};

const Notice: FC<{ icon?: ReactNode; children: ReactNode }> = ({
	icon,
	children,
}) => (
	<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
		{icon}
		<span>{children}</span>
	</div>
);
