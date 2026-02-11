import { paginatedGlobalWorkspaceSessions } from "api/queries/globalWorkspaceSessions";
import { useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { isNonInitialPage } from "components/PaginationWidget/utils";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useOrganizationsFilterMenu } from "modules/tableFiltering/options";
import type { FC } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { ConnectionLogPageView } from "./ConnectionLogPageView";

const ConnectionLogPage: FC = () => {
	const feats = useFeatureVisibility();

	// The "else false" is required if connection_log is undefined, which may
	// happen if the license is removed.
	//
	// see: https://github.com/coder/coder/issues/14798
	const isConnectionLogVisible = feats.connection_log || false;

	const { showOrganizations } = useDashboard();

	const [searchParams, setSearchParams] = useSearchParams();
	const sessionsQuery = usePaginatedQuery(
		paginatedGlobalWorkspaceSessions(searchParams),
	);
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: sessionsQuery.goToFirstPage,
	});

	const userMenu = useUserFilterMenu({
		value: filter.values.workspace_owner,
		onChange: (option) =>
			filter.update({
				...filter.values,
				workspace_owner: option?.value,
			}),
	});

	const organizationsMenu = useOrganizationsFilterMenu({
		value: filter.values.organization,
		onChange: (option) =>
			filter.update({
				...filter.values,
				organization: option?.value,
			}),
	});

	return (
		<>
			<title>{pageTitle("Connection Log")}</title>

			<ConnectionLogPageView
				sessions={sessionsQuery.data?.sessions}
				isNonInitialPage={isNonInitialPage(searchParams)}
				isConnectionLogVisible={isConnectionLogVisible}
				sessionsQuery={sessionsQuery}
				error={sessionsQuery.error}
				filterProps={{
					filter,
					error: sessionsQuery.error,
					menus: {
						user: userMenu,
						organization: showOrganizations ? organizationsMenu : undefined,
					},
				}}
			/>
		</>
	);
};

export default ConnectionLogPage;
