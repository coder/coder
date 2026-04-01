import { getErrorDetail } from "api/errors";
import * as oauth2 from "api/queries/oauth2";
import type * as TypesGen from "api/typesGenerated";
import { useAuthenticated } from "hooks";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { pageTitle } from "utils/page";
import { EditOAuth2AppPageView } from "./EditOAuth2AppPageView";

const EditOAuth2AppPage: FC = () => {
	const navigate = useNavigate();
	const { permissions } = useAuthenticated();
	const { appId } = useParams() as { appId: string };

	// When a new secret is created it is returned with the full secret.  This is
	// the only time it will be visible.  The secret list only returns a truncated
	// version of the secret (for differentiation purposes).  Once the user
	// acknowledges the secret we will clear it from the state.
	const [fullNewSecret, setFullNewSecret] =
		useState<TypesGen.OAuth2ProviderAppSecretFull>();

	const queryClient = useQueryClient();
	const appQuery = useQuery(oauth2.getApp(appId));
	const putAppMutation = useMutation(oauth2.putApp(queryClient));
	const deleteAppMutation = useMutation(oauth2.deleteApp(queryClient));
	const secretsQuery = useQuery({
		...oauth2.getAppSecrets(appId),
		enabled: permissions.viewOAuth2AppSecrets,
	});
	const postSecretMutation = useMutation(oauth2.postAppSecret(queryClient));
	const deleteSecretMutation = useMutation(oauth2.deleteAppSecret(queryClient));

	return (
		<>
			<title>{pageTitle("Edit OAuth2 Application")}</title>

			<EditOAuth2AppPageView
				app={appQuery.data}
				secrets={secretsQuery.data}
				isLoadingApp={appQuery.isLoading}
				isLoadingSecrets={secretsQuery.isLoading}
				mutatingResource={{
					updateApp: putAppMutation.isPending,
					deleteApp: deleteAppMutation.isPending,
					createSecret: postSecretMutation.isPending,
					deleteSecret: deleteSecretMutation.isPending,
				}}
				fullNewSecret={fullNewSecret}
				ackFullNewSecret={() => setFullNewSecret(undefined)}
				error={
					appQuery.error ||
					putAppMutation.error ||
					deleteAppMutation.error ||
					secretsQuery.error ||
					postSecretMutation.error ||
					deleteSecretMutation.error
				}
				updateApp={async (req) => {
					const mutation = putAppMutation.mutateAsync(
						{ id: appId, req },
						{
							onSuccess: () => {
								navigate("/deployment/oauth2-provider/apps?updated=true");
							},
						},
					);
					toast.promise(mutation, {
						success: `Successfully updated the OAuth2 application "${req.name}".`,
						error: (error) => ({
							message: `Failed to update "${req.name}" OAuth2 application.`,
							description: getErrorDetail(error),
						}),
					});
				}}
				deleteApp={async (name) => {
					const mutation = deleteAppMutation.mutateAsync(appId, {
						onSuccess: () => {
							toast.success(
								`You have successfully deleted the "${name}" OAuth2 application.`,
							);
							navigate("/deployment/oauth2-provider/apps?deleted=true");
						},
					});
					toast.promise(mutation, {
						success: `You have successfully deleted the "${name}" OAuth2 application.`,
						error: (error) => ({
							message: `Failed to delete "${name}" OAuth2 application.`,
							description: getErrorDetail(error),
						}),
					});
				}}
				generateAppSecret={async () => {
					const mutation = postSecretMutation.mutateAsync(appId, {
						onSuccess: (secret) => {
							setFullNewSecret(secret);
						},
					});
					toast.promise(mutation, {
						success: "Successfully generated OAuth2 client secret.",
						error: (error) => ({
							message: "Failed to generate OAuth2 client secret.",
							description: getErrorDetail(error),
						}),
					});
				}}
				deleteAppSecret={async (secretId: string) => {
					const mutation = deleteSecretMutation.mutateAsync(
						{ appId, secretId },
						{
							onSuccess: () => {
								if (fullNewSecret?.id === secretId) {
									setFullNewSecret(undefined);
								}
							},
						},
					);
					toast.promise(mutation, {
						success: "Successfully deleted an OAuth2 client secret.",
						error: (error) => ({
							message: "Failed to delete OAuth2 client secret.",
							description: getErrorDetail(error),
						}),
					});
				}}
				canEditApp={permissions.editOAuth2App}
				canDeleteApp={permissions.deleteOAuth2App}
				canViewAppSecrets={permissions.viewOAuth2AppSecrets}
			/>
		</>
	);
};

export default EditOAuth2AppPage;
