import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { regenerateUserSSHKey, userSSHKey } from "#/api/queries/sshKeys";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Section } from "../Section";
import { SSHKeysPageView } from "./SSHKeysPageView";

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
			<Section title="SSH keys">
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
				confirmLoading={regenerateSSHKeyMutation.isPending}
				title="Regenerate SSH key?"
				description="You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces."
				confirmText="Confirm"
				onClose={() => setIsConfirmingRegeneration(false)}
				onConfirm={async () => {
					try {
						await regenerateSSHKeyMutation.mutateAsync();
						toast.success("SSH Key regenerated successfully.");
					} catch (error) {
						toast.error(
							getErrorMessage(error, "Failed to regenerate SSH key"),
							{
								description: getErrorDetail(error),
							},
						);
					} finally {
						setIsConfirmingRegeneration(false);
					}
				}}
			/>
		</>
	);
};

export default SSHKeysPage;
