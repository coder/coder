import { getAuditLogs } from "api/api"
import { assign, createMachine } from "xstate"

type AuditLogs = Awaited<ReturnType<typeof getAuditLogs>>

export const auditMachine = createMachine(
  {
    id: "auditMachine",
    schema: {
      context: {} as { auditLogs: AuditLogs },
      services: {} as {
        loadAuditLogs: {
          data: AuditLogs
        }
      },
    },
    tsTypes: {} as import("./auditXService.typegen").Typegen0,
    states: {
      loadingLogs: {
        invoke: {
          src: "loadAuditLogs",
          onDone: {
            target: "success",
            actions: ["assignAuditLogs"],
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
      assignAuditLogs: assign({
        auditLogs: (_, event) => event.data,
      }),
    },
    services: {
      loadAuditLogs: () => getAuditLogs(),
    },
  },
)
