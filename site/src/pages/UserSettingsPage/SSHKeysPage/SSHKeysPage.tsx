import { useMachine } from "@xstate/react";
import { PropsWithChildren, FC } from "react";
import { sshKeyMachine } from "xServices/sshKey/sshKeyXService";
import { ConfirmDialog } from "../../../components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Section } from "../../../components/SettingsLayout/Section";
import { SSHKeysPageView } from "./SSHKeysPageView";

export const Language = {
  title: "SSH keys",
  regenerateDialogTitle: "Regenerate SSH key?",
  regenerateDialogMessage:
    "You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
  confirmLabel: "Confirm",
  cancelLabel: "Cancel",
};

export const SSHKeysPage: FC<PropsWithChildren<unknown>> = () => {
  const [sshState, sshSend] = useMachine(sshKeyMachine);
  const isLoading = sshState.matches("gettingSSHKey");
  const hasLoaded = sshState.matches("loaded");
  const { getSSHKeyError, regenerateSSHKeyError, sshKey } = sshState.context;

  const onRegenerateClick = () => {
    sshSend({ type: "REGENERATE_SSH_KEY" });
  };

  return (
    <>
      <Section title={Language.title}>
        <SSHKeysPageView
          isLoading={isLoading}
          hasLoaded={hasLoaded}
          getSSHKeyError={getSSHKeyError}
          regenerateSSHKeyError={regenerateSSHKeyError}
          sshKey={sshKey}
          onRegenerateClick={onRegenerateClick}
        />
      </Section>

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={sshState.matches("confirmSSHKeyRegenerate")}
        confirmLoading={sshState.matches("regeneratingSSHKey")}
        title={Language.regenerateDialogTitle}
        confirmText={Language.confirmLabel}
        onConfirm={() => {
          sshSend({ type: "CONFIRM_REGENERATE_SSH_KEY" });
        }}
        onClose={() => {
          sshSend({ type: "CANCEL_REGENERATE_SSH_KEY" });
        }}
        description={<>{Language.regenerateDialogMessage}</>}
      />
    </>
  );
};

export default SSHKeysPage;
