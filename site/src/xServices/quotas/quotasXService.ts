import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { WorkspaceQuota } from "../../api/typesGenerated"

export type QuotaContext = {
  quota?: WorkspaceQuota
  getQuotaError?: Error | unknown
}

export type QuotaEvent = {
  type: "GET_QUOTA"
  username: string
}

export const quotaMachine = createMachine(
  {
    id: "quotasMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./quotasXService.typegen").Typegen0,
    schema: {
      context: {} as QuotaContext,
      events: {} as QuotaEvent,
      services: {
        getQuota: {
          data: {} as WorkspaceQuota,
        },
      },
    },
    context: {},
    initial: "idle",
    states: {
      idle: {
        on: {
          GET_QUOTA: "gettingQuotas",
        },
      },
      gettingQuotas: {
        entry: "clearGetQuotaError",
        invoke: {
          id: "getQuota",
          src: "getQuota",
          onDone: {
            target: "success",
            actions: ["assignQuota"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetQuotaError"],
          },
        },
      },
      success: {
        type: "final",
      },
    },
  },
  {
    actions: {
      assignQuota: assign({
        quota: (_, event) => event.data,
      }),
      assignGetQuotaError: assign({
        getQuotaError: (_, event) => event.data,
      }),
      clearGetQuotaError: assign({
        getQuotaError: (_) => undefined,
      }),
    },
    services: {
      getQuota: (context, event) => API.getWorkspaceQuota(event.username),
    },
  },
)
