/**
 * @fileoverview workspaceSchedule is an xstate machine backing a form to CRUD
 * an individual workspace's schedule.
 */
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ApiError, FieldErrors, mapApiErrorToFieldErrors } from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  errorSubmissionFailed: "Failed to update schedule",
  errorWorkspaceFetch: "Failed to fetch workspace",
  successMessage: "Successfully updated workspace schedule.",
}

export interface WorkspaceScheduleContext {
  formErrors?: FieldErrors
  getWorkspaceError?: Error | unknown
  /**
   * Each workspace has their own schedule (start and ttl). For this reason, we
   * re-fetch the workspace to ensure we're up-to-date. As a result, this
   * machine is partially influenced by workspaceXService.
   */
  workspace?: TypesGen.Workspace
}

export type WorkspaceScheduleEvent =
  | { type: "GET_WORKSPACE"; workspaceId: string }
  | {
      type: "SUBMIT_SCHEDULE"
      autoStart: TypesGen.UpdateWorkspaceAutostartRequest
      ttl: TypesGen.UpdateWorkspaceTTLRequest
    }

export const workspaceSchedule = createMachine(
  {
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
    id: "workspaceScheduleState",
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
            target: "presentForm",
            actions: ["assignWorkspace"],
          },
          onError: {
            target: "error",
            actions: ["assignGetWorkspaceError", "displayWorkspaceError"],
          },
        },
        tags: "loading",
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
            actions: ["assignSubmissionError", "displaySubmissionError"],
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
        formErrors: (_, event) => mapApiErrorToFieldErrors((event.data as ApiError).response.data),
      }),
      assignWorkspace: assign({
        workspace: (_, event) => event.data,
      }),
      assignGetWorkspaceError: assign({
        getWorkspaceError: (_, event) => event.data,
      }),
      clearContext: () => {
        assign({ workspace: undefined })
      },
      clearGetWorkspaceError: (context) => {
        assign({ ...context, getWorkspaceError: undefined })
      },
      displayWorkspaceError: () => {
        displayError(Language.errorWorkspaceFetch)
      },
      displaySubmissionError: () => {
        displayError(Language.errorSubmissionFailed)
      },
      displaySuccess: () => {
        displaySuccess(Language.successMessage)
      },
    },

    services: {
      getWorkspace: async (_, event) => {
        return await API.getWorkspace(event.workspaceId)
      },
      submitSchedule: async (context, event) => {
        if (!context.workspace?.id) {
          // This state is theoretically impossible, but helps TS
          throw new Error("failed to load workspace")
        }

        // REMARK: These calls are purposefully synchronous because if one
        //         value contradicts the other, we don't want a race condition
        //         on re-submission.
        await API.putWorkspaceAutostart(context.workspace.id, event.autoStart)
        await API.putWorkspaceAutostop(context.workspace.id, event.ttl)
      },
    },
  },
)
