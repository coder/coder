import { getAuditLogs } from "api/api"
import { getErrorMessage } from "api/errors"
import { AuditLog, AuditLogResponse } from "api/typesGenerated"
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
  apiError?: Error | unknown
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
          data: AuditLogResponse
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
        entry: ["clearPreviousAuditLogs", "clearError"],
        invoke: {
          src: "loadAuditLogsAndCount",
          onDone: {
            target: "idle",
            actions: ["assignAuditLogsAndCount"],
          },
          onError: {
            target: "idle",
            actions: ["displayApiError", "assignError"],
          },
        },
        onDone: "idle",
      },
      idle: {
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
    },
  },
  {
    actions: {
      clearPreviousAuditLogs: assign({
        auditLogs: (_) => undefined,
      }),
      assignAuditLogsAndCount: assign({
        auditLogs: (_, event) => event.data.audit_logs,
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
      assignError: assign({
        apiError: (_, event) => event.data,
      }),
      clearError: assign({
        apiError: (_) => undefined,
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
          return getAuditLogs({
            offset,
            limit,
            q: context.filter,
          })
        } else {
          throw new Error("Cannot get audit logs without pagination data")
        }
      },
    },
  },
)
