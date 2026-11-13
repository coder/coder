import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { regenerateUserSSHKey, userSSHKey } from "#/api/queries/sshKeys";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { toast } from "#/components/Toaster/toast";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { SSHKeysPageView } from "./SSHKeysPageView";

const SSHKeysPage: React.FC = () => {
	const [isConfirmingRegeneration, setIsConfirmingRegeneration] =
		useState(false);

	const userSSHKeyQuery = useQuery(userSSHKey("me"));
	const queryClient = useQueryClient();
	const regenerateSSHKeyMutation = useMutation(
		regenerateUserSSHKey("me", queryClient),
	);

	const { experiments } = useDashboard();
	const showSignCommits = experiments.includes("git-commit-signing");
	const preferencesQuery = useQuery({
		...preferenceSettings(),
		enabled: showSignCommits,
	});
	const updatePreferencesMutation = useMutation(
		updatePreferenceSettings(queryClient),
	);

	return (
		<>
			<title>{pageTitle("SSH Keys", "Settings")}</title>
			<SettingsHeader>
				<SettingsHeaderTitle>SSH keys</SettingsHeaderTitle>
			</SettingsHeader>
			<SSHKeysPageView
				isLoading={userSSHKeyQuery.isLoading}
				getSSHKeyError={userSSHKeyQuery.error}
				sshKey={userSSHKeyQuery.data}
				onRegenerateClick={() => setIsConfirmingRegeneration(true)}
				showSignCommits={showSignCommits}
				signCommitsError={preferencesQuery.error}
				signCommits={preferencesQuery.data?.sign_commits_with_ssh_key ?? false}
				isSignCommitsDisabled={
					!preferencesQuery.data || updatePreferencesMutation.isPending
				}
				onSignCommitsChange={(checked) =>
					updatePreferencesMutation.mutate(
						{ sign_commits_with_ssh_key: checked },
						{
							onError: (error) =>
								toast.error(
									getErrorMessage(error, "Failed to update commit signing"),
									{ description: getErrorDetail(error) },
								),
						},
					)
				}
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
