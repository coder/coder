import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated"

type LogsContext = {
  // Build
  username: string
  workspaceName: string
  buildNumber: string
  buildId: string
  // Used to reference logs before + after.
  timeCursor: Date
  build?: WorkspaceBuild
  getBuildError?: Error | unknown
  // Logs
  logs?: ProvisionerJobLog[]
}

type LogsEvent = {
  type: "ADD_LOG"
  log: ProvisionerJobLog
}

export const workspaceBuildMachine = createMachine(
  {
    id: "workspaceBuildState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceBuildXService.typegen").Typegen0,
    schema: {
      context: {} as LogsContext,
      events: {} as LogsEvent,
      services: {} as {
        getWorkspaceBuild: {
          data: WorkspaceBuild
        }
        getLogs: {
          data: ProvisionerJobLog[]
        }
      },
    },
    initial: "gettingBuild",
    states: {
      gettingBuild: {
        entry: "clearGetBuildError",
        invoke: {
          src: "getWorkspaceBuild",
          onDone: {
            target: "logs",
            actions: ["assignBuild", "assignBuildId"],
          },
          onError: {
            target: "idle",
            actions: "assignGetBuildError",
          },
        },
      },
      idle: {},
      logs: {
        initial: "gettingExistentLogs",
        states: {
          gettingExistentLogs: {
            invoke: {
              id: "getLogs",
              src: "getLogs",
              onDone: {
                actions: ["assignLogs"],
                target: "watchingLogs",
              },
            },
          },
          watchingLogs: {
            id: "watchingLogs",
            invoke: {
              id: "streamWorkspaceBuildLogs",
              src: "streamWorkspaceBuildLogs",
            },
          },
          loaded: {
            type: "final",
          },
        },
        on: {
          ADD_LOG: {
            actions: "addLog",
          },
        },
      },
    },
  },
  {
    actions: {
      // Build ID
      assignBuildId: assign({
        buildId: (_, event) => event.data.id,
      }),
      // Build
      assignBuild: assign({
        build: (_, event) => event.data,
      }),
      assignGetBuildError: assign({
        getBuildError: (_, event) => event.data,
      }),
      clearGetBuildError: assign({
        getBuildError: (_) => undefined,
      }),
      // Logs
      assignLogs: assign({
        logs: (_, event) => event.data,
      }),
      addLog: assign({
        logs: (context, event) => {
          const previousLogs = context.logs ?? []
          return [...previousLogs, event.log]
        },
      }),
    },
    services: {
      getWorkspaceBuild: (ctx) =>
        API.getWorkspaceBuildByNumber(
          ctx.username,
          ctx.workspaceName,
          ctx.buildNumber,
        ),
      getLogs: async (ctx) =>
        API.getWorkspaceBuildLogs(ctx.buildId, ctx.timeCursor),
      streamWorkspaceBuildLogs: (ctx) => async (callback) => {
        return new Promise<void>((resolve, reject) => {
          if (!ctx.logs) {
            return reject("logs must be set")
          }
          const proto = location.protocol === "https:" ? "wss:" : "ws:"
          const socket = new WebSocket(
            `${proto}//${location.host}/api/v2/workspacebuilds/${
              ctx.buildId
            }/logs?follow=true&after=${ctx.logs[ctx.logs.length - 1].id}`,
          )
          socket.binaryType = "blob"
          socket.addEventListener("message", (event) => {
            callback({ type: "ADD_LOG", log: JSON.parse(event.data) })
          })
          socket.addEventListener("error", () => {
            reject(new Error("socket errored"))
          })
          socket.addEventListener("open", () => {
            resolve()
          })
          socket.addEventListener("close", () => {
            // When the socket closes, logs have finished streaming!
            resolve()
          })
        })
      },
    },
  },
)
