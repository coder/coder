import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { getSettings, paginatedApps, putSettings } from "#/api/queries/oauth2";
import { useFilter } from "#/components/Filter/Filter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { pageTitle } from "#/utils/page";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const OAuth2AppsSettingsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const appsQuery = usePaginatedQuery(paginatedApps(searchParams));
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: appsQuery.goToFirstPage,
	});

	const canCreateApp = permissions.createOAuth2App;
	// Gates the query and the prop below. Spelled once because a disabled query
	// reports `isLoading: false` with no data, so the two drifting apart would
	// render the tab permanently on its absent-value branch, with no spinner and
	// no error to explain it.
	const canViewSettings = permissions.viewDeploymentConfig;
	const canEditSettings = permissions.editDeploymentConfig;

	const settingsQuery = useQuery({
		...getSettings(),
		enabled: canViewSettings,
	});
	const updateSettingsMutation = useMutation(putSettings(queryClient));

	return (
		<>
			<title>{pageTitle("OAuth2 applications")}</title>

			<OAuth2AppsSettingsPageView
				apps={appsQuery.data?.apps}
				appsQuery={appsQuery}
				filter={filter}
				isLoadingApps={appsQuery.isLoading}
				appsError={appsQuery.error}
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
