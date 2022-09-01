import { getAuditLogs, getAuditLogsCount } from "api/api"
import { getErrorMessage } from "api/errors"
import { displayError } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

type AuditLogs = Awaited<ReturnType<typeof getAuditLogs>>

export const auditMachine = createMachine(
  {
    id: "auditMachine",
    schema: {
      context: {} as { auditLogs?: AuditLogs; count?: number; page: number; limit: number },
      services: {} as {
        loadAuditLogs: {
          data: AuditLogs
        }
        loadAuditLogsCount: {
          data: number
        }
      },
      events: {} as
        | {
            type: "NEXT"
          }
        | {
            type: "PREVIOUS"
          }
        | {
            type: "GO_TO_PAGE"
            page: number
          },
    },
    tsTypes: {} as import("./auditXService.typegen").Typegen0,
    initial: "loading",
    states: {
      loading: {
        invoke: [
          {
            src: "loadAuditLogs",
            onDone: {
              actions: ["assignAuditLogs"],
            },
            onError: {
              target: "error",
              actions: ["displayLoadAuditLogsError"],
            },
          },
          {
            src: "loadAuditLogsCount",
            onDone: {
              actions: ["assignCount"],
            },
            onError: {
              target: "error",
              actions: ["displayLoadAuditLogsCountError"],
            },
          },
        ],
        onDone: "success",
      },
      success: {
        on: {
          NEXT: {
            actions: ["assignNextPage"],
            target: "loading",
          },
          PREVIOUS: {
            actions: ["assignPreviousPage"],
            target: "loading",
          },
          GO_TO_PAGE: {
            actions: ["assignPage"],
            target: "loading",
          },
        },
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
      assignCount: assign({
        count: (_, event) => event.data,
      }),
      assignNextPage: assign({
        page: ({ page }) => page + 1,
      }),
      assignPreviousPage: assign({
        page: ({ page }) => page - 1,
      }),
      assignPage: assign({
        page: ({ page }) => page,
      }),
      displayLoadAuditLogsError: (_, event) => {
        const message = getErrorMessage(event.data, "Error on loading audit logs.")
        displayError(message)
      },
      displayLoadAuditLogsCountError: (_, event) => {
        const message = getErrorMessage(event.data, "Error on loading number of audit log entries.")
        displayError(message)
      },
    },
    services: {
      loadAuditLogs: ({ page, limit }, _) =>
        getAuditLogs({
          // The page in the API starts at 0
          offset: (page - 1) * limit,
          limit,
        }),
      loadAuditLogsCount: () => getAuditLogsCount(),
    },
  },
)
