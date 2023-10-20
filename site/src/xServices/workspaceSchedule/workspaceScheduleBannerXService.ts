/**
 * @fileoverview workspaceScheduleBanner is an xstate machine backing a form,
 * presented as an Alert/banner, for reactively updating a workspace schedule.
 */
import { getErrorMessage } from "api/errors";
import { Workspace } from "api/typesGenerated";
import dayjs from "dayjs";
import minMax from "dayjs/plugin/minMax";
import { getDeadline, getMaxDeadline, getMinDeadline } from "utils/schedule";
import { assign, createMachine } from "xstate";
import * as API from "api/api";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";

dayjs.extend(minMax);

export const Language = {
  errorExtension: "Failed to update workspace shutdown time.",
  successExtension: "Updated workspace shutdown time.",
};

export interface WorkspaceScheduleBannerContext {
  workspace: Workspace;
}

export type WorkspaceScheduleBannerEvent =
  | {
      type: "INCREASE_DEADLINE";
      hours: number;
    }
  | {
      type: "DECREASE_DEADLINE";
      hours: number;
    }
  | {
      type: "REFRESH_WORKSPACE";
      workspace: Workspace;
    };

export const workspaceScheduleBannerMachine = createMachine(
  {
    id: "workspaceScheduleBannerState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceScheduleBannerXService.typegen").Typegen0,
    schema: {
      events: {} as WorkspaceScheduleBannerEvent,
      context: {} as WorkspaceScheduleBannerContext,
    },
    initial: "idle",
    on: {
      REFRESH_WORKSPACE: { actions: "assignWorkspace" },
    },
    states: {
      idle: {
        on: {
          INCREASE_DEADLINE: "increasingDeadline",
          DECREASE_DEADLINE: "decreasingDeadline",
        },
      },
      increasingDeadline: {
        invoke: {
          src: "increaseDeadline",
          id: "increaseDeadline",
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
      decreasingDeadline: {
        invoke: {
          src: "decreaseDeadline",
          id: "decreaseDeadline",
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
        displayError(getErrorMessage(event.data, Language.errorExtension));
      },
      displaySuccessMessage: () => {
        displaySuccess(Language.successExtension);
      },
      assignWorkspace: assign((_, event) => ({
        workspace: event.workspace,
      })),
    },

    services: {
      increaseDeadline: async (context, event) => {
        if (!context.workspace.latest_build.deadline) {
          throw Error("Deadline is undefined.");
        }
        const proposedDeadline = getDeadline(context.workspace).add(
          event.hours,
          "hours",
        );
        const newDeadline = dayjs.min(
          proposedDeadline,
          getMaxDeadline(context.workspace),
        );
        await API.putWorkspaceExtension(context.workspace.id, newDeadline);
      },
      decreaseDeadline: async (context, event) => {
        if (!context.workspace.latest_build.deadline) {
          throw Error("Deadline is undefined.");
        }
        const proposedDeadline = getDeadline(context.workspace).subtract(
          event.hours,
          "hours",
        );
        const newDeadline = dayjs.max(proposedDeadline, getMinDeadline());
        await API.putWorkspaceExtension(context.workspace.id, newDeadline);
      },
    },
  },
);
