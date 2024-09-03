import { watchWorkspaceAgentLogs } from "api/api";
import { agentLogs } from "api/queries/workspaces";
import type {
	WorkspaceAgentLifecycle,
	WorkspaceAgentLog,
} from "api/typesGenerated";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "react-query";

export type UseAgentLogsOptions = Readonly<{
	workspaceId: string;
	agentId: string;
	agentLifeCycleState: WorkspaceAgentLifecycle;
	enabled?: boolean;
}>;

/**
 * Defines a custom hook that gives you all workspace agent logs for a given
 * workspace.Depending on the status of the workspace, all logs may or may not
 * be available.
 */
export function useAgentLogs(
	options: UseAgentLogsOptions,
): readonly WorkspaceAgentLog[] | undefined {
	const { workspaceId, agentId, agentLifeCycleState, enabled = true } = options;
	const queryClient = useQueryClient();
	const queryOptions = agentLogs(workspaceId, agentId);
	const { data: logs, isFetched } = useQuery({ ...queryOptions, enabled });

	// Track the ID of the last log received when the initial logs response comes
	// back. If the logs are not complete, the ID will mark the start point of the
	// Web sockets response so that the remaining logs can be received over time
	const lastQueriedLogId = useRef(0);
	useEffect(() => {
		const isAlreadyTracking = lastQueriedLogId.current !== 0;
		if (isAlreadyTracking) {
			return;
		}

		const lastLog = logs?.at(-1);
		if (lastLog !== undefined) {
			lastQueriedLogId.current = lastLog.id;
		}
	}, [logs]);

	const addLogs = useEffectEvent((newLogs: WorkspaceAgentLog[]) => {
		queryClient.setQueryData(
			queryOptions.queryKey,
			(oldData: WorkspaceAgentLog[] = []) => [...oldData, ...newLogs],
		);
	});

	useEffect(() => {
		// Stream data only for new logs. Old logs should be loaded beforehand
		// using a regular fetch to avoid overloading the websocket with all
		// logs at once.
		if (!isFetched) {
			return;
		}

		// If the agent is off, we don't need to stream logs. This is the only state
		// where the Coder API can't receive logs for the agent from third-party
		// apps like envbuilder.
		if (agentLifeCycleState === "off") {
			return;
		}

		const socket = watchWorkspaceAgentLogs(agentId, {
			after: lastQueriedLogId.current,
			onMessage: (newLogs) => {
				// Prevent new logs getting added when a connection is not open
				if (socket.readyState !== WebSocket.OPEN) {
					return;
				}
				addLogs(newLogs);
			},
			onError: (error) => {
				// For some reason Firefox and Safari throw an error when a websocket
				// connection is close in the middle of a message and because of that we
				// can't safely show to the users an error message since most of the
				// time they are just internal stuff. This does not happen to Chrome at
				// all and I tried to find better way to "soft close" a WS connection on
				// those browsers without success.
				console.error(error);
			},
		});

		return () => {
			socket.close();
		};
	}, [addLogs, agentId, agentLifeCycleState, isFetched]);

	return logs;
}
