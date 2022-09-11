import { getAuditLogs, getAuditLogsCount } from "api/api"
import { getErrorMessage } from "api/errors"
import { AuditLog } from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

export const auditMachine = createMachine(
  {
    id: "auditMachine",
    schema: {
      context: {} as { auditLogs?: AuditLog[]; count?: number; page: number; limit: number },
      services: {} as {
        loadAuditLogsAndCount: {
          data: {
            auditLogs: AuditLog[]
            count: number
          }
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
        // Right now, XState doesn't a good job with state + context typing so
        // this forces the AuditPageView to showing the loading state when the
        // loading state is called again by cleaning up the audit logs data
        entry: "clearPreviousAuditLogs",
        invoke: {
          src: "loadAuditLogsAndCount",
          onDone: {
            target: "success",
            actions: ["assignAuditLogsAndCount"],
          },
          onError: {
            target: "error",
            actions: ["displayApiError"],
          },
        },
        onDone: "success",
      },
      success: {
        on: {
          NEXT: {
            actions: ["assignNextPage", "onPageChange"],
            target: "loading",
          },
          PREVIOUS: {
            actions: ["assignPreviousPage", "onPageChange"],
            target: "loading",
          },
          GO_TO_PAGE: {
            actions: ["assignPage", "onPageChange"],
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
      clearPreviousAuditLogs: assign({
        auditLogs: (_) => undefined,
      }),
      assignAuditLogsAndCount: assign({
        auditLogs: (_, event) => event.data.auditLogs,
        count: (_, event) => event.data.count,
      }),
      assignNextPage: assign({
        page: ({ page }) => page + 1,
      }),
      assignPreviousPage: assign({
        page: ({ page }) => page - 1,
      }),
      assignPage: assign({
        page: (_, { page }) => page,
      }),
      displayApiError: (_, event) => {
        const message = getErrorMessage(event.data, "Error on loading audit logs.")
        displayError(message)
      },
    },
    services: {
      loadAuditLogsAndCount: async ({ page, limit }, _) => {
        const [auditLogs, count] = await Promise.all([
          getAuditLogs({
            // The page in the API starts at 0
            offset: (page - 1) * limit,
            limit,
          }).then((data) => data.audit_logs),
          getAuditLogsCount().then((data) => data.count),
        ])

        return {
          auditLogs,
          count,
        }
      },
    },
  },
)
