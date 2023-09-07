import { assign, createMachine } from "xstate";
import * as API from "api/api";
import { UpdateUserPasswordRequest } from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { t } from "i18next";

interface Context {
  userId: string;
  error?: unknown;
}

type Events = { type: "UPDATE_SECURITY"; data: UpdateUserPasswordRequest };

export const userSecuritySettingsMachine = createMachine(
  {
    id: "userSecuritySettings",
    predictableActionArguments: true,
    schema: {
      context: {} as Context,
      events: {} as Events,
    },
    tsTypes: {} as import("./userSecuritySettingsXService.typegen").Typegen0,
    initial: "idle",
    states: {
      idle: {
        on: {
          UPDATE_SECURITY: {
            target: "updatingSecurity",
          },
        },
      },
      updatingSecurity: {
        entry: "clearError",
        invoke: {
          src: "updateSecurity",
          onDone: [
            {
              actions: ["notifyUpdate", "redirectToHome"],
              target: "idle",
            },
          ],
          onError: [
            {
              actions: "assignError",
              target: "idle",
            },
          ],
        },
      },
    },
  },
  {
    services: {
      updateSecurity: async ({ userId }, { data }) =>
        API.updateUserPassword(userId, data),
    },
    actions: {
      clearError: assign({
        error: (_) => undefined,
      }),
      notifyUpdate: () => {
        displaySuccess(
          t("securityUpdateSuccessMessage", { ns: "userSettingsPage" }),
        );
      },
      assignError: assign({
        error: (_, event) => event.data,
      }),
      redirectToHome: () => {
        window.location.href = location.origin;
      },
    },
  },
);
