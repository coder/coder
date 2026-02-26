import { getErrorDetail, getErrorMessage } from "api/errors";
import { getApps, revokeApp } from "api/queries/oauth2";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { useAuthenticated } from "hooks";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { Section } from "../Section";
import OAuth2ProviderPageView from "./OAuth2ProviderPageView";

const OAuth2ProviderPage: FC = () => {
	const { user: me } = useAuthenticated();
	const queryClient = useQueryClient();
	const userOAuth2AppsQuery = useQuery(getApps(me.id));
	const revokeAppMutation = useMutation(revokeApp(queryClient, me.id));
	const [appIdToRevoke, setAppIdToRevoke] = useState<string>();
	const appToRevoke = userOAuth2AppsQuery.data?.find(
		(app) => app.id === appIdToRevoke,
	);

	return (
		<Section title="OAuth2 Applications" layout="fluid">
			<OAuth2ProviderPageView
				isLoading={userOAuth2AppsQuery.isLoading}
				error={userOAuth2AppsQuery.error}
				apps={userOAuth2AppsQuery.data}
				revoke={(app) => {
					setAppIdToRevoke(app.id);
				}}
			/>
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
							toast.success(
								`OAuth2 application "${appToRevoke.name}" revoked successfully.`,
							);
							setAppIdToRevoke(undefined);
						} catch (error) {
							toast.error(
								getErrorMessage(error, "Failed to revoke application."),
								{
									description: getErrorDetail(error),
								},
							);
						}
					}}
				/>
			)}
		</Section>
	);
};

export default OAuth2ProviderPage;
