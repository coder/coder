import { PropsWithChildren, FC, useState } from "react";
import { ConfirmDialog } from "../../../components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Section } from "../../../components/SettingsLayout/Section";
import { SSHKeysPageView } from "./SSHKeysPageView";
import { useRegenerateUserSSHKey, useUserSSHKey } from "api/queries/sshKeys";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export const Language = {
  title: "SSH keys",
  regenerateDialogTitle: "Regenerate SSH key?",
  regenerateDialogMessage:
    "You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
  confirmLabel: "Confirm",
  cancelLabel: "Cancel",
};

export const SSHKeysPage: FC<PropsWithChildren<unknown>> = () => {
  const [isConfirmingRegeneration, setIsConfirmingRegeneration] =
    useState(false);
  const userSSHKeyQuery = useUserSSHKey("me");
  const regenerateUserSSHKeyMutation = useRegenerateUserSSHKey("me", () => {
    displaySuccess("SSH Key regenerated successfully.");
    setIsConfirmingRegeneration(false);
  });

  return (
    <>
      <Section title={Language.title}>
        <SSHKeysPageView
          isLoading={userSSHKeyQuery.isLoading}
          getSSHKeyError={userSSHKeyQuery.error}
          regenerateSSHKeyError={regenerateUserSSHKeyMutation.error}
          sshKey={userSSHKeyQuery.data}
          onRegenerateClick={() => {
            setIsConfirmingRegeneration(true);
          }}
        />
      </Section>

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={isConfirmingRegeneration}
        confirmLoading={regenerateUserSSHKeyMutation.isLoading}
        title={Language.regenerateDialogTitle}
        confirmText={Language.confirmLabel}
        onConfirm={() => {
          regenerateUserSSHKeyMutation.mutate();
        }}
        onClose={() => {
          setIsConfirmingRegeneration(false);
        }}
        description={<>{Language.regenerateDialogMessage}</>}
      />
    </>
  );
};

export default SSHKeysPage;
