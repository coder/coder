import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	createUserSecret,
	deleteUserSecret,
	importUserSecrets,
	updateUserSecret,
	userSecrets,
} from "#/api/queries/userSecrets";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useUserSecretFilePathEnabled } from "#/hooks/useEmbeddedMetadata";
import { SecretsPageView } from "./SecretsPageView";
import { buildImportSuccessMessage } from "./secretForm";

const SecretsPage: FC = () => {
	const { user: me } = useAuthenticated();
	const queryClient = useQueryClient();
	const filePathEnabled = useUserSecretFilePathEnabled();
	const secretsQueryOptions = userSecrets(me.id);
	const secretsQuery = useQuery(secretsQueryOptions);
	const createSecretMutation = useMutation(
		createUserSecret(queryClient, me.id),
	);
	const updateSecretMutation = useMutation(
		updateUserSecret(queryClient, me.id),
	);
	const deleteSecretMutation = useMutation(
		deleteUserSecret(queryClient, me.id),
	);
	const importSecretsMutation = useMutation(
		importUserSecrets(queryClient, me.id),
	);

	return (
		<SecretsPageView
			secrets={secretsQuery.data}
			filePathEnabled={filePathEnabled}
			isLoading={!secretsQuery.isFetched && secretsQuery.isFetching}
			hasLoaded={secretsQuery.isSuccess}
			isCreating={createSecretMutation.isPending}
			isUpdating={updateSecretMutation.isPending}
			isDeleting={deleteSecretMutation.isPending}
			getSecretsError={secretsQuery.error}
			onCreateSecret={async (request) => {
				const secret = await createSecretMutation.mutateAsync(request);
				toast.success(`Created secret "${secret.name}" successfully.`);
				return secret;
			}}
			onUpdateSecret={async (name, request) => {
				const secret = await updateSecretMutation.mutateAsync({
					name,
					request,
				});
				toast.success(`Updated secret "${secret.name}" successfully.`);
				return secret;
			}}
			onImportSecrets={async (request) => {
				const secrets = await importSecretsMutation.mutateAsync(request);
				toast.success(buildImportSuccessMessage(secrets));
				return secrets;
			}}
			onDeleteSecret={async (secret) => {
				try {
					await deleteSecretMutation.mutateAsync(secret.name);
					toast.success(`Deleted secret "${secret.name}" successfully.`);
				} catch (error) {
					toast.error(getErrorMessage(error, "Failed to delete secret."), {
						description: getErrorDetail(error),
					});
					throw error;
				}
			}}
			onToggleSecretEnabled={async (secret, enabled) => {
				try {
					await updateSecretMutation.mutateAsync({
						name: secret.name,
						request: { enabled },
					});
					toast.success(
						`${enabled ? "Enabled" : "Disabled"} secret "${secret.name}".`,
					);
				} catch (error) {
					toast.error(
						getErrorMessage(
							error,
							`Failed to ${enabled ? "enable" : "disable"} secret.`,
						),
						{ description: getErrorDetail(error) },
					);
					throw error;
				}
			}}
		/>
	);
};

export default SecretsPage;
