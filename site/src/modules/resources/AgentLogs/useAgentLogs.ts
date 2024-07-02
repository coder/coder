import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "react-query";
import { watchWorkspaceAgentLogs } from "api/api";
import { agentLogs } from "api/queries/workspaces";
import type {
  WorkspaceAgentLifecycle,
  WorkspaceAgentLog,
} from "api/typesGenerated";
import { useEffectEvent } from "hooks/hookPolyfills";

export type UseAgentLogsOptions = Readonly<{
  workspaceId: string;
  agentId: string;
  agentLifeCycleState: WorkspaceAgentLifecycle;
  enabled?: boolean;
}>;

/**
 * Defines a custom hook that gives you all workspace agent logs for a given
 * workspace.
 *
 * Depending on the status of the workspace, all logs may or may not be
 * available.
 */
export function useAgentLogs(
  options: UseAgentLogsOptions,
): readonly WorkspaceAgentLog[] | undefined {
  const { workspaceId, agentId, agentLifeCycleState, enabled = true } = options;

  const queryClient = useQueryClient();
  const queryOptions = agentLogs(workspaceId, agentId);
  const { data: logs, isFetched } = useQuery({ ...queryOptions, enabled });

  const lastQueriedLogId = useRef(0);
  useEffect(() => {
    const lastLog = logs?.at(-1);
    const canSetLogId = lastLog !== undefined && lastQueriedLogId.current === 0;

    if (canSetLogId) {
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
    if (agentLifeCycleState !== "starting" || !isFetched) {
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
