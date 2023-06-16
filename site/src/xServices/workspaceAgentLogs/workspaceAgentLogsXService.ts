import * as API from "api/api"
import { createMachine, assign } from "xstate"
import * as TypesGen from "api/typesGenerated"
import { Line } from "components/Logs/Logs"

// Logs are stored as the Line interface to make rendering
// much more efficient. Instead of mapping objects each time, we're
// able to just pass the array of logs to the component.
export interface LineWithIDAndEOF extends Line {
  id: number
  eof: boolean
}

export const workspaceAgentLogsMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "workspaceAgentLogsMachine",
    schema: {
      events: {} as
        | {
            type: "ADD_STARTUP_LOGS"
            logs: LineWithIDAndEOF[]
          }
        | {
            type: "FETCH_STARTUP_LOGS"
          }
        | {
            type: "STARTUP_DONE"
          },
      context: {} as {
        agentID: string
        startupLogs?: LineWithIDAndEOF[]
      },
      services: {} as {
        getStartupLogs: {
          data: LineWithIDAndEOF[]
        }
      },
    },
    tsTypes: {} as import("./workspaceAgentLogsXService.typegen").Typegen0,
    initial: "waiting",
    states: {
      waiting: {
        on: {
          FETCH_STARTUP_LOGS: "loading",
        },
      },
      loading: {
        invoke: {
          src: "getStartupLogs",
          onDone: {
            target: "watchStartupLogs",
            actions: ["assignStartupLogs"],
          },
        },
      },
      watchStartupLogs: {
        id: "watchingStartupLogs",
        invoke: {
          id: "streamStartupLogs",
          src: "streamStartupLogs",
        },
        on: {
          ADD_STARTUP_LOGS: {
            actions: "addStartupLogs",
          },
          STARTUP_DONE: {
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
      getStartupLogs: (ctx) =>
        API.getWorkspaceAgentStartupLogs(ctx.agentID).then((data) =>
          data.map((log) => ({
            id: log.id,
            level: log.level || "info",
            output: log.output,
            time: log.created_at,
            eof: log.eof,
          })),
        ),
      streamStartupLogs: (ctx) => async (callback) => {
        let after = 0
        if (ctx.startupLogs && ctx.startupLogs.length > 0) {
          after = ctx.startupLogs[ctx.startupLogs.length - 1].id
        }

        const socket = API.watchStartupLogs(ctx.agentID, {
          after,
          onMessage: (logs) => {
            callback({
              type: "ADD_STARTUP_LOGS",
              logs: logs.map((log) => ({
                id: log.id,
                level: "info" as TypesGen.LogLevel,
                output: log.output,
                time: log.created_at,
                eof: log.eof,
              })),
            })
          },
          onDone: () => {
            callback({ type: "STARTUP_DONE" })
          },
          onError: (error) => {
            console.error(error)
          },
        })

        return () => {
          socket.close()
        }
      },
    },
    actions: {
      assignStartupLogs: assign({
        startupLogs: (_, { data }) => data,
      }),
      addStartupLogs: assign({
        startupLogs: (context, event) => {
          const previousLogs = context.startupLogs ?? []
          return [...previousLogs, ...event.logs]
        },
      }),
    },
  },
)
