import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated"

type LogsContext = {
  // Build
  buildId: string
  build?: WorkspaceBuild
  getBuildError?: Error | unknown
  // Logs
  logs?: ProvisionerJobLog[]
  getBuildLogsError?: Error | unknown
}

export const workspaceBuildMachine = createMachine(
  {
    id: "workspaceBuildState",
    schema: {
      context: {} as LogsContext,
      services: {} as {
        getWorkspaceBuild: {
          data: WorkspaceBuild
        }
        getWorkspaceBuildLogs: {
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
        initial: "gettingLogs",
        states: {
          gettingLogs: {
            entry: "clearGetBuildLogsError",
            invoke: {
              src: "getWorkspaceBuildLogs",
              onDone: {
                target: "idle",
                actions: "assignLogs",
              },
              onError: {
                target: "idle",
                actions: "assignGetBuildLogsError",
              },
            },
          },
          idle: {},
        },
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
      assignGetBuildLogsError: assign({
        getBuildLogsError: (_, event) => event.data,
      }),
      clearGetBuildLogsError: assign({
        getBuildLogsError: (_) => undefined,
      }),
    },
    services: {
      getWorkspaceBuild: (ctx) => API.getWorkspaceBuild(ctx.buildId),
      getWorkspaceBuildLogs: (ctx) => API.getWorkspaceBuildLogs(ctx.buildId),
    },
  },
)
