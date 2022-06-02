/**
 * @fileoverview workspaceScheduleBanner is an xstate machine backing a form,
 * presented as an Alert/banner, for reactively extending a workspace schedule.
 */
import { createMachine } from "xstate"
import * as API from "../../api/api"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"
import { defaultWorkspaceExtension } from "../../util/workspace"

export const Language = {
  errorExtension: "Failed to extend workspace deadline.",
  successExtension: "Successfully extended workspace deadline.",
}

export type WorkspaceScheduleBannerEvent = { type: "EXTEND_DEADLINE_DEFAULT"; workspaceId: string }

export const workspaceScheduleBannerMachine = createMachine(
  {
    tsTypes: {} as import("./workspaceScheduleBannerXService.typegen").Typegen0,
    schema: {
      events: {} as WorkspaceScheduleBannerEvent,
    },
    id: "workspaceScheduleBannerState",
    initial: "idle",
    states: {
      idle: {
        on: {
          EXTEND_DEADLINE_DEFAULT: "extendingDeadline",
        },
      },
      extendingDeadline: {
        invoke: {
          src: "extendDeadlineDefault",
          id: "extendDeadlineDefault",
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
      displayFailureMessage: () => {
        displayError(Language.errorExtension)
      },
      displaySuccessMessage: () => {
        displaySuccess(Language.successExtension)
      },
    },

    services: {
      extendDeadlineDefault: async (_, event) => {
        await API.putWorkspaceExtension(event.workspaceId, defaultWorkspaceExtension())
      },
    },
  },
)
