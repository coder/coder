/**
 * @fileoverview workspaceSchedule is an xstate machine backing a form to CRUD
 * an individual workspace's schedule.
 */
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"
import { displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  successMessage: "Successfully updated workspace schedule.",
}

type Permissions = Record<keyof ReturnType<typeof permissionsToCheck>, boolean>

export interface WorkspaceScheduleContext {
  getWorkspaceError?: Error | unknown
  /**
   * Each workspace has their own schedule (start and ttl). For this reason, we
   * re-fetch the workspace to ensure we're up-to-date. As a result, this
   * machine is partially influenced by workspaceXService.
   */
  workspace?: TypesGen.Workspace
  permissions?: Permissions
  checkPermissionsError?: Error | unknown
  submitScheduleError?: Error | unknown
}

export const checks = {
  updateWorkspace: "updateWorkspace",
} as const

const permissionsToCheck = (workspace: TypesGen.Workspace) => ({
  [checks.updateWorkspace]: {
    object: {
      resource_type: "workspace",
      resource_id: workspace.id,
      owner_id: workspace.owner_id,
    },
    action: "update",
  },
})

export type WorkspaceScheduleEvent =
  | { type: "GET_WORKSPACE"; username: string; workspaceName: string }
  | {
      type: "SUBMIT_SCHEDULE"
      autoStart: TypesGen.UpdateWorkspaceAutostartRequest | undefined
      ttl: TypesGen.UpdateWorkspaceTTLRequest
    }

export const workspaceSchedule = createMachine(
  {
    id: "workspaceScheduleState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceScheduleXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceScheduleContext,
      events: {} as WorkspaceScheduleEvent,
      services: {} as {
        getWorkspace: {
          data: TypesGen.Workspace
        }
      },
    },
    initial: "idle",
    on: {
      GET_WORKSPACE: "gettingWorkspace",
    },
    states: {
      idle: {
        tags: "loading",
      },
      gettingWorkspace: {
        entry: ["clearGetWorkspaceError", "clearContext"],
        invoke: {
          src: "getWorkspace",
          id: "getWorkspace",
          onDone: {
            target: "gettingPermissions",
            actions: ["assignWorkspace"],
          },
          onError: {
            target: "error",
            actions: ["assignGetWorkspaceError"],
          },
        },
        tags: "loading",
      },
      gettingPermissions: {
        entry: "clearGetPermissionsError",
        invoke: {
          src: "checkPermissions",
          id: "checkPermissions",
          onDone: [
            {
              actions: ["assignPermissions"],
              target: "presentForm",
            },
          ],
          onError: [
            {
              actions: "assignGetPermissionsError",
              target: "error",
            },
          ],
        },
      },
      presentForm: {
        on: {
          SUBMIT_SCHEDULE: "submittingSchedule",
        },
      },
      submittingSchedule: {
        invoke: {
          src: "submitSchedule",
          id: "submitSchedule",
          onDone: {
            target: "submitSuccess",
            actions: "displaySuccess",
          },
          onError: {
            target: "presentForm",
            actions: ["assignSubmissionError"],
          },
        },
        tags: "loading",
      },
      submitSuccess: {
        on: {
          SUBMIT_SCHEDULE: "submittingSchedule",
        },
      },
      error: {
        on: {
          GET_WORKSPACE: "gettingWorkspace",
        },
      },
    },
  },
  {
    actions: {
      assignSubmissionError: assign({
        submitScheduleError: (_, event) => event.data,
      }),
      assignWorkspace: assign({
        workspace: (_, event) => event.data,
      }),
      assignGetWorkspaceError: assign({
        getWorkspaceError: (_, event) => event.data,
      }),
      assignPermissions: assign({
        // Setting event.data as Permissions to be more stricted. So we know
        // what permissions we asked for.
        permissions: (_, event) => event.data as Permissions,
      }),
      assignGetPermissionsError: assign({
        checkPermissionsError: (_, event) => event.data,
      }),
      clearGetPermissionsError: assign({
        checkPermissionsError: (_) => undefined,
      }),
      clearContext: () => {
        assign({ workspace: undefined, permissions: undefined })
      },
      clearGetWorkspaceError: (context) => {
        assign({ ...context, getWorkspaceError: undefined })
      },
      displaySuccess: () => {
        displaySuccess(Language.successMessage)
      },
    },

    services: {
      getWorkspace: async (_, event) => {
        return await API.getWorkspaceByOwnerAndName(
          event.username,
          event.workspaceName,
        )
      },
      checkPermissions: async (context) => {
        if (context.workspace) {
          return await API.checkAuthorization({
            checks: permissionsToCheck(context.workspace),
          })
        } else {
          throw Error(
            "Cannot check permissions without both workspace and user id",
          )
        }
      },
      submitSchedule: async (context, event) => {
        if (!context.workspace?.id) {
          // This state is theoretically impossible, but helps TS
          throw new Error("Failed to load workspace.")
        }

        if (event.autoStart?.schedule !== undefined) {
          await API.putWorkspaceAutostart(context.workspace.id, event.autoStart)
        }
        await API.putWorkspaceAutostop(context.workspace.id, event.ttl)
      },
    },
  },
)
