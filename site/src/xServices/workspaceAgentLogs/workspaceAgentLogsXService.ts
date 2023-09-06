import * as API from "api/api";
import { createMachine, assign } from "xstate";
import { Line } from "components/WorkspaceBuildLogs/Logs/Logs";

// Logs are stored as the Line interface to make rendering
// much more efficient. Instead of mapping objects each time, we're
// able to just pass the array of logs to the component.
export interface LineWithID extends Line {
  id: number;
}

export const workspaceAgentLogsMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "workspaceAgentLogsMachine",
    schema: {
      events: {} as
        | {
            type: "ADD_LOGS";
            logs: LineWithID[];
          }
        | {
            type: "FETCH_LOGS";
          }
        | {
            type: "DONE";
          },
      context: {} as {
        agentID: string;
        logs?: LineWithID[];
      },
      services: {} as {
        getLogs: {
          data: LineWithID[];
        };
      },
    },
    tsTypes: {} as import("./workspaceAgentLogsXService.typegen").Typegen0,
    initial: "waiting",
    states: {
      waiting: {
        on: {
          FETCH_LOGS: "loading",
        },
      },
      loading: {
        invoke: {
          src: "getLogs",
          onDone: {
            target: "watchLogs",
            actions: ["assignLogs"],
          },
        },
      },
      watchLogs: {
        id: "watchingLogs",
        invoke: {
          id: "streamLogs",
          src: "streamLogs",
        },
        on: {
          ADD_LOGS: {
            actions: "addLogs",
          },
          DONE: {
            target: "loaded",
          },
        },
      },
      loaded: {
        type: "final",
      },
    },
  },
  {
    services: {
      getLogs: (ctx) =>
        API.getWorkspaceAgentLogs(ctx.agentID).then((data) =>
          data.map((log) => ({
            id: log.id,
            level: log.level || "info",
            output: log.output,
            time: log.created_at,
          })),
        ),
      streamLogs: (ctx) => async (callback) => {
        let after = 0;
        if (ctx.logs && ctx.logs.length > 0) {
          after = ctx.logs[ctx.logs.length - 1].id;
        }

        const socket = API.watchWorkspaceAgentLogs(ctx.agentID, {
          after,
          onMessage: (logs) => {
            callback({
              type: "ADD_LOGS",
              logs: logs.map((log) => ({
                id: log.id,
                level: log.level || "info",
                output: log.output,
                time: log.created_at,
              })),
            });
          },
          onDone: () => {
            callback({ type: "DONE" });
          },
          onError: (error) => {
            console.error(error);
          },
        });

        return () => {
          socket.close();
        };
      },
    },
    actions: {
      assignLogs: assign({
        logs: (_, { data }) => data,
      }),
      addLogs: assign({
        logs: (context, event) => {
          const previousLogs = context.logs ?? [];
          return [...previousLogs, ...event.logs];
        },
      }),
    },
  },
);
