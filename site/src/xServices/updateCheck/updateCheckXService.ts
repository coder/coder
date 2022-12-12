import { assign, createMachine } from "xstate"
import { checkAuthorization, getUpdateCheck } from "api/api"
import { AuthorizationResponse, UpdateCheckResponse } from "api/typesGenerated"

export const checks = {
  viewUpdateCheck: "viewUpdateCheck",
}

export const permissionsToCheck = {
  [checks.viewUpdateCheck]: {
    object: {
      resource_type: "update_check",
    },
    action: "read",
  },
}

export type Permissions = Record<keyof typeof permissionsToCheck, boolean>

export interface UpdateCheckContext {
  show: boolean
  updateCheck?: UpdateCheckResponse
  permissions?: Permissions
  error?: Error | unknown
}

export type UpdateCheckEvent =
  | { type: "CHECK" }
  | { type: "CLEAR" }
  | { type: "DISMISS" }

export const updateCheckMachine = createMachine(
  {
    id: "updateCheckState",
    predictableActionArguments: true,
    tsTypes: {} as import("./updateCheckXService.typegen").Typegen0,
    schema: {
      context: {} as UpdateCheckContext,
      events: {} as UpdateCheckEvent,
      services: {} as {
        checkPermissions: {
          data: AuthorizationResponse
        }
        getUpdateCheck: {
          data: UpdateCheckResponse
        }
      },
    },
    context: {
      show: false,
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          CHECK: {
            target: "fetchingPermissions",
          },
        },
      },
      fetchingPermissions: {
        invoke: {
          src: "checkPermissions",
          id: "checkPermissions",
          onDone: [
            {
              actions: ["assignPermissions"],
              target: "checkingPermissions",
            },
          ],
          onError: [
            {
              actions: ["assignError"],
              target: "show",
            },
          ],
        },
      },
      checkingPermissions: {
        always: [
          {
            target: "fetchingUpdateCheck",
            cond: "canViewUpdateCheck",
          },
          {
            target: "dismissOrClear",
            cond: "canNotViewUpdateCheck",
          },
        ],
      },
      fetchingUpdateCheck: {
        invoke: {
          src: "getUpdateCheck",
          id: "getUpdateCheck",
          onDone: [
            {
              actions: ["assignUpdateCheck", "clearError"],
              target: "show",
            },
          ],
          onError: [
            {
              actions: ["assignError", "clearUpdateCheck"],
              target: "show",
            },
          ],
        },
      },
      show: {
        entry: "assignShow",
        always: [
          {
            target: "dismissOrClear",
          },
        ],
      },
      dismissOrClear: {
        on: {
          DISMISS: {
            actions: ["assignHide", "setDismissedVersion"],
            target: "dismissed",
          },
          CLEAR: {
            actions: ["clearUpdateCheck", "clearError", "assignHide"],
            target: "idle",
          },
        },
      },
      dismissed: {
        type: "final",
      },
    },
  },
  {
    services: {
      checkPermissions: async () =>
        checkAuthorization({ checks: permissionsToCheck }),
      getUpdateCheck: getUpdateCheck,
    },
    actions: {
      assignPermissions: assign({
        permissions: (_, event) => event.data as Permissions,
      }),
      assignShow: assign((context) => ({
        show:
          localStorage.getItem("dismissedVersion") !==
          context.updateCheck?.version,
      })),
      assignHide: assign({
        show: false,
      }),
      assignUpdateCheck: assign({
        updateCheck: (_, event) => event.data,
      }),
      clearUpdateCheck: assign((context) => ({
        ...context,
        updateCheck: undefined,
      })),
      assignError: assign({
        error: (_, event) => event.data,
      }),
      clearError: assign((context) => ({
        ...context,
        error: undefined,
      })),
      setDismissedVersion: (context) => {
        if (context.updateCheck?.version) {
          // We use localStorage to ensure users who have dismissed the UpdateCheckBanner are not plagued by its reappearance on page reload
          localStorage.setItem("dismissedVersion", context.updateCheck.version)
        }
      },
    },
    guards: {
      canViewUpdateCheck: (context) =>
        context.permissions?.[checks.viewUpdateCheck] || false,
      canNotViewUpdateCheck: (context) =>
        !context.permissions?.[checks.viewUpdateCheck],
    },
  },
)
