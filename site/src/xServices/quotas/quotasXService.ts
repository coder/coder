import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import { WorkspaceQuota } from "../../api/typesGenerated";

export type QuotaContext = {
  username: string;
  quota?: WorkspaceQuota;
  getQuotaError?: unknown;
};

export const quotaMachine = createMachine(
  {
    id: "quotasMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./quotasXService.typegen").Typegen0,
    schema: {
      context: {} as QuotaContext,
      services: {
        getQuota: {
          data: {} as WorkspaceQuota,
        },
      },
    },
    initial: "gettingQuotas",
    states: {
      idle: {},
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
      getQuota: ({ username }) => API.getWorkspaceQuota(username),
    },
  },
);
