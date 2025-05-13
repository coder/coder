import { watchWorkspaceAgentLogs } from "api/api";
import { agentLogs } from "api/queries/workspaces";
import type {
	WorkspaceAgent,
	WorkspaceAgentLifecycle,
	WorkspaceAgentLog,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useEffect } from "react";
import { useQuery, useQueryClient } from "react-query";

const ON_GOING_STATES: WorkspaceAgentLifecycle[] = ["starting", "created"];

export function useAgentLogs(
	agent: WorkspaceAgent,
): readonly WorkspaceAgentLog[] | undefined {
	const queryClient = useQueryClient();
	const agentLogsOptions = agentLogs(agent.id);
	const shouldUseSocket = ON_GOING_STATES.includes(agent.lifecycle_state);
	const { data: logs } = useQuery({
		...agentLogsOptions,
		enabled: !shouldUseSocket,
	});

	const appendAgentLogs = useEffectEvent(
		async (newLogs: WorkspaceAgentLog[]) => {
			await queryClient.cancelQueries(agentLogsOptions.queryKey);
			queryClient.setQueryData<WorkspaceAgentLog[]>(
				agentLogsOptions.queryKey,
				(oldLogs) => (oldLogs ? [...oldLogs, ...newLogs] : newLogs),
			);
		},
	);

	const refreshAgentLogs = useEffectEvent(() => {
		queryClient.invalidateQueries(agentLogsOptions.queryKey);
	});

	useEffect(() => {
		if (!shouldUseSocket) {
			return;
		}

		const socket = watchWorkspaceAgentLogs(agent.id, { after: 0 });
		socket.addEventListener("message", (e) => {
			if (e.parseError) {
				console.warn("Error parsing agent log: ", e.parseError);
				return;
			}
			appendAgentLogs(e.parsedMessage);
		});

		socket.addEventListener("error", (e) => {
			console.error("Error in agent log socket: ", e);
			displayError(
				"Unable to watch the agent logs",
				"Please try refreshing the browser",
			);
			socket.close();
		});

		return () => {
			socket.close();
			// For some reason, after closing the socket, a few logs still getting
			// generated in the BE. This is a workaround to avoid we don't display
			// them in the UI.
			refreshAgentLogs();
		};
	}, [agent.id, shouldUseSocket, appendAgentLogs, refreshAgentLogs]);

	return logs;
}
