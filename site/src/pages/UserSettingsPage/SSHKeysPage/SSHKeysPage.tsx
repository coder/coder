import { PropsWithChildren, FC, useState } from "react";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Section } from "components/SettingsLayout/Section";
import { SSHKeysPageView } from "./SSHKeysPageView";
import { regenerateUserSSHKey, userSSHKey } from "api/queries/sshKeys";
import { useMutation, useQuery, useQueryClient } from "react-query";

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

  const userSSHKeyQuery = useQuery(userSSHKey("me"));
  const queryClient = useQueryClient();
  const regenerateSSHKeyMutation = useMutation(
    regenerateUserSSHKey("me", queryClient),
  );

  return (
    <>
      <Section title={Language.title}>
        <SSHKeysPageView
          isLoading={userSSHKeyQuery.isLoading}
          getSSHKeyError={userSSHKeyQuery.error}
          regenerateSSHKeyError={regenerateSSHKeyMutation.error}
          sshKey={userSSHKeyQuery.data}
          onRegenerateClick={() => setIsConfirmingRegeneration(true)}
        />
      </Section>

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={isConfirmingRegeneration}
        confirmLoading={regenerateSSHKeyMutation.isLoading}
        title={Language.regenerateDialogTitle}
        description={Language.regenerateDialogMessage}
        confirmText={Language.confirmLabel}
        onClose={() => setIsConfirmingRegeneration(false)}
        onConfirm={async () => {
          try {
            await regenerateSSHKeyMutation.mutateAsync();
            displaySuccess("SSH Key regenerated successfully.");
          } catch (err) {
            // No error handling because UI displays the error message after
            // React Query automatically puts it into state
          } finally {
            setIsConfirmingRegeneration(false);
          }
        }}
      />
    </>
  );
};

export default SSHKeysPage;
