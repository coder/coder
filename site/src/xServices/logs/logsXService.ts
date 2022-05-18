import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ProvisionerJobLog } from "../../api/types"

type LogsContext = {
  buildname: string
  logs?: ProvisionerJobLog[]
  getBuildLogsError?: Error | unknown
}

export const logsMachine = createMachine(
  {
    schema: {
      context: {} as LogsContext,
      services: {} as {
        getBuildLogs: {
          data: ProvisionerJobLog[]
        }
      },
    },
    tsTypes: {} as import("./logsXService.typegen").Typegen0,
    initial: "gettingLogs",
    states: {
      gettingLogs: {
        entry: "clearGetBuildLogsError",
        invoke: {
          src: "getBuildLogs",
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
  {
    actions: {
      assignLogs: assign({
        logs: (_, event) => event.data,
      }),
      assignGetBuildLogsError: assign({
        getBuildLogsError: (_, event) => event.data,
      }),
      clearGetBuildLogsError: assign({
        logs: (_) => undefined,
      }),
    },
    services: {
      getBuildLogs: (ctx) => API.getBuildLogs(ctx.buildname),
    },
  },
)
