import { paginatedConnectionLogs } from "api/queries/connectionlog";
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
import { useStatusFilterMenu, useTypeFilterMenu } from "./ConnectionLogFilter";
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
	const connectionlogsQuery = usePaginatedQuery(
		paginatedConnectionLogs(searchParams),
	);
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: connectionlogsQuery.goToFirstPage,
	});

	const userMenu = useUserFilterMenu({
		value: filter.values.workspace_owner,
		onChange: (option) =>
			filter.update({
				...filter.values,
				workspace_owner: option?.value,
			}),
	});

	const statusMenu = useStatusFilterMenu({
		value: filter.values.status,
		onChange: (option) =>
			filter.update({
				...filter.values,
				status: option?.value,
			}),
	});

	const typeMenu = useTypeFilterMenu({
		value: filter.values.type,
		onChange: (option) =>
			filter.update({
				...filter.values,
				type: option?.value,
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
				connectionLogs={connectionlogsQuery.data?.connection_logs}
				isNonInitialPage={isNonInitialPage(searchParams)}
				isConnectionLogVisible={isConnectionLogVisible}
				connectionLogsQuery={connectionlogsQuery}
				error={connectionlogsQuery.error}
				filterProps={{
					filter,
					error: connectionlogsQuery.error,
					menus: {
						user: userMenu,
						status: statusMenu,
						type: typeMenu,
						organization: showOrganizations ? organizationsMenu : undefined,
					},
				}}
			/>
		</>
	);
};

export default ConnectionLogPage;
