import { assign, createMachine } from "xstate";
import { getUpdateCheck } from "api/api";
import { AuthorizationResponse, UpdateCheckResponse } from "api/typesGenerated";
import { checks, Permissions } from "components/AuthProvider/permissions";

export interface UpdateCheckContext {
  permissions: Permissions;
  updateCheck?: UpdateCheckResponse;
  error?: unknown;
}

export type UpdateCheckEvent = { type: "DISMISS" };

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
          data: AuthorizationResponse;
        };
        getUpdateCheck: {
          data: UpdateCheckResponse;
        };
      },
    },
    initial: "checkingPermissions",
    states: {
      checkingPermissions: {
        always: [
          {
            target: "fetchingUpdateCheck",
            cond: "canViewUpdateCheck",
          },
          {
            target: "dismissed",
          },
        ],
      },
      fetchingUpdateCheck: {
        invoke: {
          src: "getUpdateCheck",
          id: "getUpdateCheck",
          onDone: [
            {
              actions: ["assignUpdateCheck"],
              target: "show",
              cond: "shouldShowUpdateCheck",
            },
            {
              target: "dismissed",
            },
          ],
          onError: [
            {
              actions: ["assignError"],
              target: "dismissed",
            },
          ],
        },
      },
      show: {
        on: {
          DISMISS: {
            actions: ["setDismissedVersion"],
            target: "dismissed",
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
      // For some reason, when passing values directly, jest.spy does not work.
      getUpdateCheck: () => getUpdateCheck(),
    },
    actions: {
      assignUpdateCheck: assign({
        updateCheck: (_, event) => event.data,
      }),
      assignError: assign({
        error: (_, event) => event.data,
      }),
      setDismissedVersion: ({ updateCheck }) => {
        if (!updateCheck) {
          throw new Error("Update check is not set");
        }

        saveDismissedVersionOnLocal(updateCheck.version);
      },
    },
    guards: {
      canViewUpdateCheck: ({ permissions }) =>
        permissions[checks.viewUpdateCheck] || false,
      shouldShowUpdateCheck: (_, { data }) => {
        const isNotDismissed = getDismissedVersionOnLocal() !== data.version;
        const isOutdated = !data.current;
        return isNotDismissed && isOutdated;
      },
    },
  },
);

// Exporting to be used in the tests
export const saveDismissedVersionOnLocal = (version: string): void => {
  window.localStorage.setItem("dismissedVersion", version);
};

export const getDismissedVersionOnLocal = (): string | undefined => {
  return localStorage.getItem("dismissedVersion") ?? undefined;
};

export const clearDismissedVersionOnLocal = (): void => {
  localStorage.removeItem("dismissedVersion");
};
