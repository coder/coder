/**
 * @fileoverview workspaceScheduleBanner is an xstate machine backing a form,
 * presented as an Alert/banner, for reactively updating a workspace schedule.
 */
import { getErrorMessage } from "api/errors"
import { Template, Workspace } from "api/typesGenerated"
import dayjs from "dayjs"
import minMax from "dayjs/plugin/minMax"
import {
  canExtendDeadline,
  canReduceDeadline,
  getDeadline,
  getMaxDeadline,
  getMinDeadline,
} from "util/schedule"
import { ActorRefFrom, assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"

dayjs.extend(minMax)

export const Language = {
  errorExtension: "Failed to update workspace shutdown time.",
  successExtension: "Updated workspace shutdown time.",
}

export interface WorkspaceScheduleBannerContext {
  workspace: Workspace
  template: Template
  deadline?: dayjs.Dayjs
}

export type WorkspaceScheduleBannerEvent =
  | {
      type: "INCREASE_DEADLINE"
      hours: number
    }
  | {
      type: "DECREASE_DEADLINE"
      hours: number
    }
  | {
      type: "REFRESH_WORKSPACE"
      workspace: Workspace
    }

export type WorkspaceScheduleBannerMachineRef = ActorRefFrom<typeof workspaceScheduleBannerMachine>

export const workspaceScheduleBannerMachine = createMachine(
  {
    id: "workspaceScheduleBannerState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceScheduleBannerXService.typegen").Typegen0,
    schema: {
      events: {} as WorkspaceScheduleBannerEvent,
      context: {} as WorkspaceScheduleBannerContext,
    },
    initial: "initialize",
    on: {
      REFRESH_WORKSPACE: { actions: "assignWorkspace" },
    },
    states: {
      initialize: {
        always: [
          { cond: "isAtMaxDeadline", target: "atMaxDeadline" },
          { cond: "isAtMinDeadline", target: "atMinDeadline" },
          { target: "midRange" },
        ],
      },
      midRange: {
        on: {
          INCREASE_DEADLINE: "increasingDeadline",
          DECREASE_DEADLINE: "decreasingDeadline",
        },
      },
      atMaxDeadline: {
        on: {
          DECREASE_DEADLINE: "decreasingDeadline",
        },
      },
      atMinDeadline: {
        on: {
          INCREASE_DEADLINE: "increasingDeadline",
        },
      },
      increasingDeadline: {
        invoke: {
          src: "increaseDeadline",
          id: "increaseDeadline",
          onDone: [
            {
              cond: "isAtMaxDeadline",
              target: "atMaxDeadline",
              actions: "displaySuccessMessage",
            },
            {
              target: "midRange",
              actions: "displaySuccessMessage",
            },
          ],
          onError: {
            target: "midRange",
            actions: "displayFailureMessage",
          },
        },
        tags: "loading",
      },
      decreasingDeadline: {
        invoke: {
          src: "decreaseDeadline",
          id: "decreaseDeadline",
          onDone: [
            {
              cond: "isAtMinDeadline",
              target: "atMinDeadline",
              actions: "displaySuccessMessage",
            },
            {
              target: "midRange",
              actions: "displaySuccessMessage",
            },
          ],
          onError: {
            target: "midRange",
            actions: "displayFailureMessage",
          },
        },
        tags: "loading",
      },
    },
  },
  {
    guards: {
      isAtMaxDeadline: (context) =>
        context.deadline
          ? !canExtendDeadline(context.deadline, context.workspace, context.template)
          : false,
      isAtMinDeadline: (context) =>
        context.deadline ? !canReduceDeadline(context.deadline) : false,
    },
    actions: {
      // This error does not have a detail, so using the snackbar is okay
      displayFailureMessage: (_, event) => {
        displayError(getErrorMessage(event.data, Language.errorExtension))
      },
      displaySuccessMessage: () => {
        displaySuccess(Language.successExtension)
      },
      assignWorkspace: assign((_, event) => ({
        workspace: event.workspace,
        deadline: getDeadline(event.workspace),
      })),
    },

    services: {
      increaseDeadline: async (context, event) => {
        if (!context.deadline) {
          throw Error("Deadline is undefined.")
        }
        const proposedDeadline = context.deadline.add(event.hours, "hours")
        const newDeadline = dayjs.min(
          proposedDeadline,
          getMaxDeadline(context.workspace, context.template),
        )
        await API.putWorkspaceExtension(context.workspace.id, newDeadline)
      },
      decreaseDeadline: async (context, event) => {
        if (!context.deadline) {
          throw Error("Deadline is undefined.")
        }
        const proposedDeadline = context.deadline.subtract(event.hours, "hours")
        const newDeadline = dayjs.max(proposedDeadline, getMinDeadline())
        await API.putWorkspaceExtension(context.workspace.id, newDeadline)
      },
    },
  },
)
