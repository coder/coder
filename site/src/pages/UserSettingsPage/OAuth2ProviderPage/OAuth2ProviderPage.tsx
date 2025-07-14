import { getErrorMessage } from "api/errors";
import { deleteApp, getApps, revokeApp } from "api/queries/oauth2";
import type * as TypesGen from "api/typesGenerated";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useAuthenticated } from "hooks";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { Section } from "../Section";
import OAuth2ProviderPageView from "./OAuth2ProviderPageView";

const OAuth2ProviderPage: FC = () => {
	const { user: me } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	// Get authorized apps (apps that user has granted access to their account)
	const authorizedAppsQuery = useQuery(getApps({ user_id: me.id }));

	// Get owned apps (apps that the user created)
	const ownedAppsQuery = useQuery(getApps({ owner_id: me.id }));

	const revokeAppMutation = useMutation(revokeApp(queryClient, me.id));
	const deleteAppMutation = useMutation(deleteApp(queryClient));

	const [appIdToRevoke, setAppIdToRevoke] = useState<string>();
	const [appIdToDelete, setAppIdToDelete] = useState<string>();

	// Now both lists come directly from the server without client-side filtering
	const authorizedApps = authorizedAppsQuery.data || [];
	const ownedApps = ownedAppsQuery.data || [];

	const appToRevoke = authorizedApps.find((app) => app.id === appIdToRevoke);
	const appToDelete = ownedApps.find((app) => app.id === appIdToDelete);

	const handleManageOwnedApp = (app: TypesGen.OAuth2ProviderApp) => {
		navigate(`${app.id}`);
	};

	const handleDeleteOwnedApp = (app: TypesGen.OAuth2ProviderApp) => {
		setAppIdToDelete(app.id);
	};

	return (
		<Section title="OAuth2 Applications" layout="fluid">
			<OAuth2ProviderPageView
				isLoading={authorizedAppsQuery.isLoading || ownedAppsQuery.isLoading}
				error={authorizedAppsQuery.error || ownedAppsQuery.error}
				authorizedApps={authorizedApps}
				ownedApps={ownedApps}
				revoke={(app) => {
					setAppIdToRevoke(app.id);
				}}
				onManageOwnedApp={handleManageOwnedApp}
				onDeleteOwnedApp={handleDeleteOwnedApp}
			/>

			{/* Revoke authorized app dialog */}
			{appToRevoke !== undefined && (
				<DeleteDialog
					title="Revoke Application"
					verb="Revoking"
					info={`This will invalidate any tokens created by the OAuth2 application "${appToRevoke.name}".`}
					label="Name of the application to revoke"
					isOpen
					confirmLoading={revokeAppMutation.isPending}
					name={appToRevoke.name}
					entity="application"
					onCancel={() => setAppIdToRevoke(undefined)}
					onConfirm={async () => {
						try {
							await revokeAppMutation.mutateAsync(appToRevoke.id);
							displaySuccess(
								`You have successfully revoked the OAuth2 application "${appToRevoke.name}"`,
							);
							setAppIdToRevoke(undefined);
						} catch (error) {
							displayError(
								getErrorMessage(error, "Failed to revoke application."),
							);
						}
					}}
				/>
			)}

			{/* Delete owned app dialog */}
			{appToDelete !== undefined && (
				<DeleteDialog
					title="Delete Application"
					verb="Deleting"
					info={`This will permanently delete the OAuth2 application "${appToDelete.name}" and invalidate all its tokens.`}
					label="Name of the application to delete"
					isOpen
					confirmLoading={deleteAppMutation.isPending}
					name={appToDelete.name}
					entity="application"
					onCancel={() => setAppIdToDelete(undefined)}
					onConfirm={async () => {
						try {
							await deleteAppMutation.mutateAsync(appToDelete.id);
							displaySuccess(
								`You have successfully deleted the OAuth2 application "${appToDelete.name}"`,
							);
							setAppIdToDelete(undefined);
						} catch (error) {
							displayError(
								getErrorMessage(error, "Failed to delete application."),
							);
						}
					}}
				/>
			)}
		</Section>
	);
};

export default OAuth2ProviderPage;
