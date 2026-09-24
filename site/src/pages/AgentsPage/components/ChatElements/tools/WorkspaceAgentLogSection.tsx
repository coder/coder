import { LoaderIcon } from "lucide-react";
import { type FC, memo, type ReactNode, useLayoutEffect, useRef } from "react";
import { useQuery } from "react-query";
import { workspaceById } from "#/api/queries/workspaces";
import type { WorkspaceAgent, WorkspaceAgentLog } from "#/api/typesGenerated";
import { DEFAULT_LOG_LINE_SIDE_PADDING } from "#/components/Logs/Logs";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { AgentLogLine } from "#/modules/resources/AgentLogs/AgentLogLine";
import { useAgentLogs } from "#/modules/resources/useAgentLogs";
import { findWorkspaceAgent } from "#/utils/workspace";
import { useChatWorkspace } from "../../../context/ChatWorkspaceContext";
import type { ToolStatus } from "./utils";

type WorkspaceAgentLogSectionProps = {
	status: ToolStatus;
	/** Build ID from the completed tool result. */
	buildId?: string;
};

/**
 * Agent startup logs for a start or create call. Renders nothing unless
 * the call's build is the workspace's latest build and that build has
 * started the workspace.
 */
export const WorkspaceAgentLogSection: FC<WorkspaceAgentLogSectionProps> = ({
	status,
	buildId,
}) => {
	const isRunning = status === "running";
	const {
		workspaceId,
		buildId: chatBuildId,
		agentId: chatAgentId,
	} = useChatWorkspace();

	// Updated live by the workspace watch socket; do not poll.
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const workspace = workspaceQuery.data;
	const callBuildId = isRunning ? chatBuildId : buildId;
	// The agent is looked up in the latest build, so that build must be the call's.
	if (
		!workspace ||
		!callBuildId ||
		workspace.latest_build.id !== callBuildId ||
		workspace.latest_build.status !== "running"
	) {
		return null;
	}

	const agent = chatAgentId
		? findWorkspaceAgent(workspace, chatAgentId)
		: undefined;
	if (!agent) {
		return isRunning ? (
			<WaitingNotice>Waiting for workspace agent…</WaitingNotice>
		) : null;
	}

	return (
		<AgentStartupLogs key={agent.id} agent={agent} isRunning={isRunning} />
	);
};

type AgentStartupLogsProps = {
	agent: WorkspaceAgent;
	isRunning: boolean;
};

const AgentStartupLogs: FC<AgentStartupLogsProps> = ({ agent, isRunning }) => {
	const logs = useAgentLogs({ agentId: agent.id });

	const endRef = useRef<HTMLDivElement>(null);
	useLayoutEffect(() => {
		if (logs.length > 0) {
			endRef.current?.scrollIntoView({ block: "end" });
		}
	}, [logs]);

	if (logs.length === 0) {
		return isRunning ? (
			<WaitingNotice>Waiting for agent logs…</WaitingNotice>
		) : null;
	}

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
					{logs.map((log) => (
						<AgentLogRow key={log.id} log={log} />
					))}
				</div>
				<div ref={endRef} />
			</div>
		</ScrollArea>
	);
};

// Memoized so a streamed batch renders only its new lines.
const AgentLogRow = memo<{ log: WorkspaceAgentLog }>(({ log }) => (
	<AgentLogLine
		line={{
			id: log.id,
			time: log.created_at,
			output: log.output,
			level: log.level,
			sourceId: log.source_id,
		}}
		sourceIcon={null}
	/>
));

const WaitingNotice: FC<{ children: ReactNode }> = ({ children }) => (
	<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
		<LoaderIcon className="size-3 animate-spin motion-reduce:animate-none" />
		<span>{children}</span>
	</div>
);
