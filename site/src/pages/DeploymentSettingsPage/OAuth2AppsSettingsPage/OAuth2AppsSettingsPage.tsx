import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getApps, getSettings, putSettings } from "#/api/queries/oauth2";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const OAuth2AppsSettingsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const appsQuery = useQuery(getApps());
	const settingsQuery = useQuery({
		...getSettings(),
		enabled: permissions.viewDeploymentConfig,
	});
	const updateSettingsMutation = useMutation(putSettings(queryClient));

	const canCreateApp = permissions.createOAuth2App;
	const canViewSettings = permissions.viewDeploymentConfig;
	const canEditSettings = permissions.editDeploymentConfig;

	return (
		<>
			<title>{pageTitle("OAuth2 applications")}</title>

			<OAuth2AppsSettingsPageView
				apps={appsQuery.data}
				isLoadingApps={appsQuery.isLoading}
				// The view gates the applications empty state on this prop, so a
				// settings failure here would claim the apps list is unknown when it
				// loaded fine.
				error={appsQuery.error}
				canCreateApp={canCreateApp}
				settings={
					canViewSettings
						? {
								canEdit: canEditSettings,
								isLoading: settingsQuery.isLoading,
								isUpdating: updateSettingsMutation.isPending,
								loadError: settingsQuery.error,
								onRetry: () => void settingsQuery.refetch(),
								updateError: updateSettingsMutation.error,
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
