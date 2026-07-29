import type { FC } from "react";
import { useSearchParams } from "react-router";
import { paginatedApps } from "#/api/queries/oauth2";
import { useFilter } from "#/components/Filter/Filter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { pageTitle } from "#/utils/page";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const OAuth2AppsSettingsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const [searchParams, setSearchParams] = useSearchParams();
	const appsQuery = usePaginatedQuery(paginatedApps(searchParams));
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: appsQuery.goToFirstPage,
	});

	const canCreateApp = permissions.createOAuth2App;

	return (
		<>
			<title>{pageTitle("OAuth2 applications")}</title>

			<OAuth2AppsSettingsPageView
				apps={appsQuery.data?.apps}
				appsQuery={appsQuery}
				filter={filter}
				isLoading={appsQuery.isLoading}
				error={appsQuery.error}
				canCreateApp={canCreateApp}
			/>
		</>
	);
};

export default OAuth2AppsSettingsPage;
