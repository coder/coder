import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated"

type LogsContext = {
  // Build
  buildId: string
  build?: WorkspaceBuild
  getBuildError?: Error | unknown
  // Logs
  logs: ProvisionerJobLog[]
}

type LogsEvent =
  | {
      type: "ADD_LOG"
      log: ProvisionerJobLog
    }
  | {
      type: "NO_MORE_LOGS"
    }

export const workspaceBuildMachine = createMachine(
  {
    id: "workspaceBuildState",
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
    tsTypes: {} as import("./workspaceBuildXService.typegen").Typegen0,
    type: "parallel",
    states: {
      build: {
        initial: "gettingBuild",
        states: {
          gettingBuild: {
            entry: "clearGetBuildError",
            invoke: {
              src: "getWorkspaceBuild",
              onDone: {
                target: "idle",
                actions: "assignBuild",
              },
              onError: {
                target: "idle",
                actions: "assignGetBuildError",
              },
            },
          },
          idle: {},
        },
      },
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
        },
        on: {
          ADD_LOG: {
            actions: "addLog",
          },
          NO_MORE_LOGS: {
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
    actions: {
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
        logs: (context, event) => [...context.logs, event.log],
      }),
    },
    services: {
      getWorkspaceBuild: (ctx) => API.getWorkspaceBuild(ctx.buildId),
      getLogs: async (ctx) => API.getWorkspaceBuildLogs(ctx.buildId),
      streamWorkspaceBuildLogs: (ctx) => async (callback) => {
        const reader = await API.streamWorkspaceBuildLogs(ctx.buildId)

        // Watching for the stream
        // eslint-disable-next-line no-constant-condition
        while (true) {
          const { value, done } = await reader.read()

          if (done) {
            callback("NO_MORE_LOGS")
            break
          }

          callback({ type: "ADD_LOG", log: value })
        }
      },
    },
  },
)
