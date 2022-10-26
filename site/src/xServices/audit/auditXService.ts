import { getAuditLogs, getAuditLogsCount } from "api/api"
import { getErrorMessage } from "api/errors"
import { AuditLog } from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { getPaginationData } from "components/PaginationWidget/utils"
import {
  PaginationContext,
  PaginationMachineRef,
  paginationMachine,
} from "xServices/pagination/paginationXService"
import { assign, createMachine, spawn, send } from "xstate"

const auditPaginationId = "auditPagination"

interface AuditContext {
  auditLogs?: AuditLog[]
  count?: number
  filter: string
  paginationContext: PaginationContext
  paginationRef?: PaginationMachineRef
}

export const auditMachine = createMachine(
  {
    id: "auditMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./auditXService.typegen").Typegen0,
    schema: {
      context: {} as AuditContext,
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
            type: "UPDATE_PAGE"
            page: string
          }
        | {
            type: "FILTER"
            filter: string
          },
    },
    initial: "startPagination",
    states: {
      startPagination: {
        entry: "assignPaginationRef",
        always: "loading",
      },
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
          UPDATE_PAGE: {
            actions: ["updateURL"],
            target: "loading",
          },
          FILTER: {
            actions: ["assignFilter", "sendResetPage"],
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
      assignPaginationRef: assign({
        paginationRef: (context) =>
          spawn(
            paginationMachine.withContext(context.paginationContext),
            auditPaginationId,
          ),
      }),
      assignFilter: assign({
        filter: (_, { filter }) => filter,
      }),
      displayApiError: (_, event) => {
        const message = getErrorMessage(
          event.data,
          "Error on loading audit logs.",
        )
        displayError(message)
      },
      sendResetPage: send({ type: "RESET_PAGE" }, { to: auditPaginationId }),
    },
    services: {
      loadAuditLogsAndCount: async (context) => {
        if (context.paginationRef) {
          const { offset, limit } = getPaginationData(context.paginationRef)
          const [auditLogs, count] = await Promise.all([
            getAuditLogs({
              offset,
              limit,
              q: context.filter,
            }).then((data) => data.audit_logs),
            getAuditLogsCount({
              q: context.filter,
            }).then((data) => data.count),
          ])

          return {
            auditLogs,
            count,
          }
        } else {
          throw new Error("Cannot get audit logs without pagination data")
        }
      },
    },
  },
)
