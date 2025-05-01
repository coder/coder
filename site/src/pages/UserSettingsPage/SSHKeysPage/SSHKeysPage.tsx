import { getErrorMessage } from "api/errors";
import { regenerateUserSSHKey, userSSHKey } from "api/queries/sshKeys";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Section } from "../Section";
import { SSHKeysPageView } from "./SSHKeysPageView";

export const Language = {
	title: "SSH keys",
	regenerateDialogTitle: "Regenerate SSH key?",
	regenerationError: "Failed to regenerate SSH key",
	regenerationSuccess: "SSH Key regenerated successfully.",
	regenerateDialogMessage:
		"You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
	confirmLabel: "Confirm",
	cancelLabel: "Cancel",
};

const SSHKeysPage: FC = () => {
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
						displaySuccess(Language.regenerationSuccess);
					} catch (error) {
						displayError(getErrorMessage(error, Language.regenerationError));
					} finally {
						setIsConfirmingRegeneration(false);
					}
				}}
			/>
		</>
	);
};

export default SSHKeysPage;
