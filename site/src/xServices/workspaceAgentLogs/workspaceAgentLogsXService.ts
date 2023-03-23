import * as API from "api/api"
import { createMachine, assign } from "xstate"
import * as TypesGen from "api/typesGenerated"
import { Line } from "components/Logs/Logs"

// Logs are stored as the Line interface to make rendering
// much more efficient. Instead of mapping objects each time, we're
// able to just pass the array of logs to the component.
export interface LineWithID extends Line {
  id: number
}

export const workspaceAgentLogsMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "workspaceAgentLogsMachine",
    schema: {
      events: {} as
        | {
            type: "ADD_STARTUP_LOGS"
            logs: LineWithID[]
          }
        | {
            type: "FETCH_STARTUP_LOGS"
          },
      context: {} as {
        agentID: string
        startupLogs?: LineWithID[]
      },
      services: {} as {
        getStartupLogs: {
          data: LineWithID[]
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
      },
      loaded: {
        type: "final",
      },
    },
    on: {
      ADD_STARTUP_LOGS: {
        actions: "addStartupLogs",
      },
    },
  },
  {
    services: {
      getStartupLogs: (ctx) =>
        API.getWorkspaceAgentStartupLogs(ctx.agentID).then((data) =>
          data.map((log) => ({
            id: log.id,
            level: "info" as TypesGen.LogLevel,
            output: log.output,
            time: log.created_at,
          })),
        ),
      streamStartupLogs: (ctx) => async (callback) => {
        return new Promise<void>((resolve, reject) => {
          const proto = location.protocol === "https:" ? "wss:" : "ws:"
          let after = 0
          if (ctx.startupLogs && ctx.startupLogs.length > 0) {
            after = ctx.startupLogs[ctx.startupLogs.length - 1].id
          }
          const socket = new WebSocket(
            `${proto}//${location.host}/api/v2/workspaceagents/${ctx.agentID}/startup-logs?follow&after=${after}`,
          )
          socket.binaryType = "blob"
          socket.addEventListener("message", (event) => {
            const logs = JSON.parse(
              event.data,
            ) as TypesGen.WorkspaceAgentStartupLog[]
            callback({
              type: "ADD_STARTUP_LOGS",
              logs: logs.map((log) => ({
                id: log.id,
                level: "info" as TypesGen.LogLevel,
                output: log.output,
                time: log.created_at,
              })),
            })
          })
          socket.addEventListener("error", () => {
            reject(new Error("socket errored"))
          })
          socket.addEventListener("open", () => {
            resolve()
          })
        })
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
