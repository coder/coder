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

export function useAgentLogs(
  options: UseAgentLogsOptions,
): readonly WorkspaceAgentLog[] | undefined {
  const { workspaceId, agentId, agentLifeCycleState, enabled = true } = options;
  const queryClient = useQueryClient();
  const queryOptions = agentLogs(workspaceId, agentId);

  const { data } = useQuery({
    ...queryOptions,
    enabled,
    select: (logs) => {
      return {
        logs,
        lastLogId: logs.at(-1)?.id ?? 0,
      };
    },
  });

  const socketRef = useRef<WebSocket | null>(null);
  const lastInitializedAgentIdRef = useRef<string | null>(null);

  const addLogs = useEffectEvent((newLogs: WorkspaceAgentLog[]) => {
    queryClient.setQueryData(
      queryOptions.queryKey,
      (oldLogs: WorkspaceAgentLog[] = []) => [...oldLogs, ...newLogs],
    );
  });

  useEffect(() => {
    const isSameAgentId = agentId === lastInitializedAgentIdRef.current;
    if (!isSameAgentId) {
      socketRef.current?.close();
    }

    const cannotCreateSocket =
      agentLifeCycleState !== "starting" || data === undefined;
    if (cannotCreateSocket) {
      return;
    }

    const socket = watchWorkspaceAgentLogs(agentId, {
      after: data.lastLogId,
      onMessage: (newLogs) => {
        // Prevent new logs getting added when a connection is not open
        if (socket.readyState === WebSocket.OPEN) {
          addLogs(newLogs);
        }
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

    socketRef.current = socket;
    lastInitializedAgentIdRef.current = agentId;
  }, [addLogs, agentId, agentLifeCycleState, data]);

  // The above effect is likely going to run a lot because we don't know when or
  // how agentLifeCycleState will change over time (it's a union of nine
  // values). The only way to ensure that we only close when we unmount is by
  // putting the logic into a separate effect with an empty dependency array
  useEffect(() => {
    const closeSocketOnUnmount = () => socketRef.current?.close();
    return closeSocketOnUnmount;
  }, []);

  return data?.logs;
}
