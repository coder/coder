import { getAuditLogs } from "api/api"
import { getErrorMessage } from "api/errors"
import { displayError } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

type AuditLogs = Awaited<ReturnType<typeof getAuditLogs>>

export const auditMachine = createMachine(
  {
    id: "auditMachine",
    schema: {
      context: {} as { auditLogs?: AuditLogs },
      services: {} as {
        loadAuditLogs: {
          data: AuditLogs
        }
      },
    },
    tsTypes: {} as import("./auditXService.typegen").Typegen0,
    initial: "loadingLogs",
    states: {
      loadingLogs: {
        invoke: {
          src: "loadAuditLogs",
          onDone: {
            target: "success",
            actions: ["assignAuditLogs"],
          },
          onError: {
            target: "error",
            actions: ["displayLoadAuditLogsError"],
          },
        },
      },
      success: {
        type: "final",
      },
      error: {
        type: "final",
      },
    },
  },
  {
    actions: {
      assignAuditLogs: assign({
        auditLogs: (_, event) => event.data,
      }),
      displayLoadAuditLogsError: (_, event) => {
        const message = getErrorMessage(event.data, "Error on loading audit logs.")
        displayError(message)
      },
    },
    services: {
      loadAuditLogs: () => getAuditLogs(),
    },
  },
)
