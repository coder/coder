import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	getApps,
	getOAuth2ProviderSettings,
	putOAuth2ProviderSettings,
} from "#/api/queries/oauth2";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const OAuth2AppsSettingsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const appsQuery = useQuery(getApps());
	const settingsQuery = useQuery({
		...getOAuth2ProviderSettings(),
		enabled: permissions.viewDeploymentConfig,
	});
	const updateSettingsMutation = useMutation(
		putOAuth2ProviderSettings(queryClient),
	);

	const canCreateApp = permissions.createOAuth2App;
	const canViewSettings = permissions.viewDeploymentConfig;
	const canEditSettings = permissions.editDeploymentConfig;

	return (
		<>
			<title>{pageTitle("OAuth2 applications")}</title>

			<OAuth2AppsSettingsPageView
				apps={appsQuery.data}
				isLoading={appsQuery.isLoading}
				// Only the apps error. The view gates the applications empty state on
				// this prop, so a settings failure here would claim the apps list is
				// unknown when it loaded fine.
				error={appsQuery.error}
				canCreateApp={canCreateApp}
				settings={
					canViewSettings
						? {
								canEdit: canEditSettings,
								isLoading: settingsQuery.isLoading,
								isUpdating: updateSettingsMutation.isPending,
								error: settingsQuery.error ?? updateSettingsMutation.error,
								dynamicClientRegistrationEnabled:
									settingsQuery.data?.dynamic_client_registration_enabled,
								onDynamicClientRegistrationChange: (enabled) => {
									updateSettingsMutation.mutate({
										dynamic_client_registration_enabled: enabled,
									});
								},
							}
						: undefined
				}
			/>
		</>
	);
};

export default OAuth2AppsSettingsPage;
