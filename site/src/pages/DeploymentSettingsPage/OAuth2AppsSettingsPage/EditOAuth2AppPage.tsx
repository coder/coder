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
					try {
						await putAppMutation.mutateAsync({ id: appId, req });
						// REVIEW: Maybe it is better to stay on the same page?
						toast.success(
							`Successfully updated the OAuth2 application "${req.name}".`,
						);
						navigate("/deployment/oauth2-provider/apps?updated=true");
					} catch (error) {
						toast.error("Failed to update OAuth2 application", {
							description: getErrorDetail(error),
						});
					}
				}}
				deleteApp={async (name) => {
					try {
						await deleteAppMutation.mutateAsync(appId);
						toast.success(
							`You have successfully deleted the OAuth2 application "${name}"`,
						);
						navigate("/deployment/oauth2-provider/apps?deleted=true");
					} catch (error) {
						toast.error("Failed to delete OAuth2 application", {
							description: getErrorDetail(error),
						});
					}
				}}
				generateAppSecret={async () => {
					try {
						const secret = await postSecretMutation.mutateAsync(appId);
						toast.success("Successfully generated OAuth2 client secret");
						setFullNewSecret(secret);
					} catch (error) {
						toast.error("Failed to generate OAuth2 client secret", {
							description: getErrorDetail(error),
						});
					}
				}}
				deleteAppSecret={async (secretId: string) => {
					try {
						await deleteSecretMutation.mutateAsync({ appId, secretId });
						toast.success("Successfully deleted an OAuth2 client secret");
						if (fullNewSecret?.id === secretId) {
							setFullNewSecret(undefined);
						}
					} catch (error) {
						toast.error("Failed to delete OAuth2 client secret", {
							description: getErrorDetail(error),
						});
					}
				}}
				canEditApp={permissions.editOAuth2App}
				canDeleteApp={permissions.deleteOAuth2App}
				canViewAppSecrets={permissions.viewOAuth2AppSecrets}
			/>
		</>
	);
};

export default EditOAuth2AppPage;
