import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated"

type LogsContext = {
  // Build
  username: string
  workspaceName: string
  buildNumber: string
  buildId: string
  build?: WorkspaceBuild
  getBuildError?: Error | unknown
  // Logs
  logs?: ProvisionerJobLog[]
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
      },
    },
    tsTypes: {} as import("./workspaceBuildXService.typegen").Typegen0,
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
        initial: "watchingLogs",
        states: {
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
          NO_MORE_LOGS: {
            target: "logs.loaded",
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
      addLog: assign({
        logs: (context, event) => {
          const previousLogs = context.logs ?? []
          return [...previousLogs, event.log]
        },
      }),
    },
    services: {
      getWorkspaceBuild: (ctx) => API.getWorkspaceBuildByNumber(ctx.username, ctx.workspaceName, ctx.buildNumber),
      streamWorkspaceBuildLogs: (ctx) => async (callback) => {
        const reader = await API.streamWorkspaceBuildLogs(ctx.buildId)

        // Watching for the stream
        // eslint-disable-next-line no-constant-condition, @typescript-eslint/no-unnecessary-condition
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
