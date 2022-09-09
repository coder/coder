/**
 * @fileoverview workspaceScheduleBanner is an xstate machine backing a form,
 * presented as an Alert/banner, for reactively updating a workspace schedule.
 */
import { getErrorMessage } from "api/errors"
import dayjs from "dayjs"
import { createMachine } from "xstate"
import * as API from "../../api/api"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  errorExtension: "Failed to update workspace shutdown time.",
  successExtension: "Updated workspace shutdown time.",
}

export type WorkspaceScheduleBannerEvent = {
  type: "UPDATE_DEADLINE"
  workspaceId: string
  newDeadline: dayjs.Dayjs
}

export const workspaceScheduleBannerMachine = createMachine(
  {
    id: "workspaceScheduleBannerState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceScheduleBannerXService.typegen").Typegen0,
    schema: {
      events: {} as WorkspaceScheduleBannerEvent,
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          UPDATE_DEADLINE: "updatingDeadline",
        },
      },
      updatingDeadline: {
        invoke: {
          src: "updateDeadline",
          id: "updateDeadline",
          onDone: {
            target: "idle",
            actions: "displaySuccessMessage",
          },
          onError: {
            target: "idle",
            actions: "displayFailureMessage",
          },
        },
        tags: "loading",
      },
    },
  },
  {
    actions: {
      // This error does not have a detail, so using the snackbar is okay
      displayFailureMessage: (_, event) => {
        displayError(getErrorMessage(event.data, Language.errorExtension))
      },
      displaySuccessMessage: () => {
        displaySuccess(Language.successExtension)
      },
    },

    services: {
      updateDeadline: async (_, event) => {
        await API.putWorkspaceExtension(event.workspaceId, event.newDeadline)
      },
    },
  },
)
