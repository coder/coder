import { useEffect, useLayoutEffect, useRef } from "react";
import { useQuery } from "react-query";
import { workspaceById } from "#/api/queries/workspaces";
import { Logs, LogsHeader } from "#/components/Logs/Logs";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useWorkspaceBuildLogs } from "#/hooks/useWorkspaceBuildLogs";
import {
	AGENT_LOG_LINE_HEIGHT,
	AgentLogOutput,
} from "#/modules/resources/AgentLogs/AgentLogLine";
import { useAgentLogs } from "#/modules/resources/useAgentLogs";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { useChatWorkspace } from "../../../context/ChatWorkspaceContext";
import { getWorkspaceAgent } from "../../ChatConversation/chatHelpers";
import { LogNotice } from "./LogNotice";
import type { ToolStatus } from "./utils";

type WorkspaceLogBoxProps = {
	status: ToolStatus;
	/** Build ID from the finished tool result. */
	buildId?: string;
	action: "create" | "start" | "stop";
	/** Shown above the agent startup log. */
	notice?: string;
};

/**
 * Build log and, for start and create calls, agent startup log of one
 * workspace tool call, in a single scroll box. The box follows new lines
 * while its viewport is at the bottom and only ever scrolls itself, so
 * streaming logs never move the chat transcript.
 */
export const WorkspaceLogBox: React.FC<WorkspaceLogBoxProps> = ({
	status,
	buildId,
	action,
	notice,
}) => {
	const isRunning = status === "running";
	const {
		workspaceId,
		buildId: chatBuildId,
		agentId: chatAgentId,
	} = useChatWorkspace();
	const callBuildId = isRunning ? chatBuildId : buildId;

	// The stream replays every log line, so it serves finished builds too.
	const buildLogs = useWorkspaceBuildLogs(callBuildId, Boolean(callBuildId));

	// Updated live by the workspace watch socket; do not poll.
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const workspace = workspaceQuery.data;
	// The backend does not wait for an agent after a stop build, and the
	// agent lookup searches only the latest build.
	const showAgent =
		action !== "stop" &&
		workspace !== undefined &&
		callBuildId !== undefined &&
		workspace.latest_build.id === callBuildId &&
		workspace.latest_build.status === "running";
	const agent = showAgent
		? getWorkspaceAgent(workspace, chatAgentId)
		: undefined;
	const agentLogs = useAgentLogs({
		agentId: agent?.id ?? "",
		enabled: Boolean(agent),
	});

	const viewportRef = useRef<HTMLDivElement>(null);
	// Starts true so the box opens at the bottom of the log.
	const isAtBottomRef = useRef(true);
	useEffect(() => {
		const viewport = viewportRef.current;
		if (!viewport) {
			return;
		}
		const onScroll = () => {
			const distanceFromBottom =
				viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight;
			isAtBottomRef.current = distanceFromBottom <= AGENT_LOG_LINE_HEIGHT;
		};
		viewport.addEventListener("scroll", onScroll);
		return () => viewport.removeEventListener("scroll", onScroll);
	}, []);
	// Sets scrollTop instead of calling scrollIntoView, which would also
	// scroll the transcript around the box.
	useLayoutEffect(() => {
		const viewport = viewportRef.current;
		if (viewport && isAtBottomRef.current) {
			viewport.scrollTop = viewport.scrollHeight;
		}
	}, [buildLogs, agentLogs, notice]);

	let buildPart: React.ReactNode;
	if (buildLogs && buildLogs.length > 0) {
		buildPart = (
			<WorkspaceBuildLogs
				logs={buildLogs}
				disableAutoscroll
				className="border-0 rounded-none"
			/>
		);
	} else if (callBuildId || isRunning) {
		buildPart = <LogNotice icon="loading">Loading build logs…</LogNotice>;
	} else {
		buildPart = <LogNotice>No build logs available.</LogNotice>;
	}

	let agentPart: React.ReactNode;
	if (agent && agentLogs.length > 0) {
		agentPart = (
			<div className="font-mono">
				<LogsHeader title="Agent startup" detail={agent.name} />
				<Logs
					lines={agentLogs.map((log) => ({
						id: log.id,
						time: log.created_at,
						output: log.output,
						level: log.level,
						sourceId: log.source_id,
					}))}
					LineOutput={AgentLogOutput}
					className="min-h-0"
				/>
			</div>
		);
	} else if (showAgent && isRunning) {
		agentPart = (
			<LogNotice icon="loading">
				Waiting for workspace agent startup to complete…
			</LogNotice>
		);
	}

	return (
		<ScrollArea
			className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
			viewportClassName="max-h-64"
			viewportTabIndex={0}
			viewportAriaLabel="Workspace log"
			viewportRef={viewportRef}
			scrollBarClassName="w-1.5"
		>
			{buildPart}
			{notice && <LogNotice icon="info">{notice}</LogNotice>}
			{agentPart}
		</ScrollArea>
	);
};
