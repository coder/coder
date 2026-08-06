import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { regenerateUserSSHKey, userSSHKey } from "#/api/queries/sshKeys";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
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
			<SettingsHeader>
				<SettingsHeaderTitle>SSH keys</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					The following public key is used to authenticate Git in workspaces.
					You may add it to Git services (such as GitHub) that you need to
					access from your workspace. Coder configures authentication via{" "}
					<code className="rounded-sm border border-border bg-surface-secondary px-1 py-0.5 text-xs text-content-primary">
						$GIT_SSH_COMMAND
					</code>
					.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<SSHKeysPageView
				isLoading={userSSHKeyQuery.isLoading}
				getSSHKeyError={userSSHKeyQuery.error}
				sshKey={userSSHKeyQuery.data}
				onRegenerateClick={() => setIsConfirmingRegeneration(true)}
			/>

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
