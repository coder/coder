import { getUserSSHKey, regenerateUserSSHKey } from "api/api";
import { GitSSHKey } from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { createMachine, assign } from "xstate";
import { i18n } from "i18n";

const { t } = i18n;

interface Context {
  sshKey?: GitSSHKey;
  getSSHKeyError?: unknown;
  regenerateSSHKeyError?: unknown;
}

type Events =
  | { type: "REGENERATE_SSH_KEY" }
  | { type: "CONFIRM_REGENERATE_SSH_KEY" }
  | { type: "CANCEL_REGENERATE_SSH_KEY" };

export const sshKeyMachine = createMachine(
  {
    id: "sshKeyState",
    predictableActionArguments: true,
    schema: {
      context: {} as Context,
      events: {} as Events,
      services: {} as {
        getSSHKey: {
          data: GitSSHKey;
        };
        regenerateSSHKey: {
          data: GitSSHKey;
        };
      },
    },
    tsTypes: {} as import("./sshKeyXService.typegen").Typegen0,
    initial: "gettingSSHKey",
    states: {
      gettingSSHKey: {
        entry: "clearGetSSHKeyError",
        invoke: {
          src: "getSSHKey",
          onDone: [
            {
              actions: "assignSSHKey",
              target: "loaded",
            },
          ],
          onError: [
            {
              actions: "assignGetSSHKeyError",
              target: "notLoaded",
            },
          ],
        },
      },
      notLoaded: {
        type: "final",
      },
      loaded: {
        on: {
          REGENERATE_SSH_KEY: {
            target: "confirmSSHKeyRegenerate",
          },
        },
      },
      confirmSSHKeyRegenerate: {
        on: {
          CANCEL_REGENERATE_SSH_KEY: {
            target: "loaded",
          },
          CONFIRM_REGENERATE_SSH_KEY: {
            target: "regeneratingSSHKey",
          },
        },
      },
      regeneratingSSHKey: {
        entry: "clearRegenerateSSHKeyError",
        invoke: {
          src: "regenerateSSHKey",
          onDone: [
            {
              actions: ["assignSSHKey", "notifySuccessSSHKeyRegenerated"],
              target: "loaded",
            },
          ],
          onError: [
            {
              actions: "assignRegenerateSSHKeyError",
              target: "loaded",
            },
          ],
        },
      },
    },
  },
  {
    services: {
      getSSHKey: () => getUserSSHKey(),
      regenerateSSHKey: () => regenerateUserSSHKey(),
    },
    actions: {
      assignSSHKey: assign({
        sshKey: (_, { data }) => data,
      }),
      assignGetSSHKeyError: assign({
        getSSHKeyError: (_, { data }) => data,
      }),
      clearGetSSHKeyError: assign({
        getSSHKeyError: (_) => undefined,
      }),
      assignRegenerateSSHKeyError: assign({
        regenerateSSHKeyError: (_, { data }) => data,
      }),
      clearRegenerateSSHKeyError: assign({
        regenerateSSHKeyError: (_) => undefined,
      }),
      notifySuccessSSHKeyRegenerated: () => {
        displaySuccess(
          t("sshRegenerateSuccessMessage", { ns: "userSettingsPage" }),
        );
      },
    },
  },
);
