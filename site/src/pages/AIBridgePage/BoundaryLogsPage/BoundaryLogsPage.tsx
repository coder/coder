import { paginatedBoundaryNetworkAuditLogs } from "api/queries/boundarynetworkauditlog";
import { useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationsFilterMenu } from "modules/tableFiltering/options";
import type { FC } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { useActionFilterMenu } from "./filter/filter";
import { BoundaryLogsPageView } from "./BoundaryLogsPageView";

const BoundaryLogsPage: FC = () => {
	const { showOrganizations } = useDashboard();

	const [searchParams, setSearchParams] = useSearchParams();
	const boundaryLogsQuery = usePaginatedQuery(
		paginatedBoundaryNetworkAuditLogs(searchParams),
	);
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: boundaryLogsQuery.goToFirstPage,
	});

	const userMenu = useUserFilterMenu({
		value: filter.values.workspace_owner,
		onChange: (option) =>
			filter.update({
				...filter.values,
				workspace_owner: option?.value,
			}),
	});

	const actionMenu = useActionFilterMenu({
		value: filter.values.action,
		onChange: (option) =>
			filter.update({
				...filter.values,
				action: option?.value,
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
			<title>{pageTitle("Boundary Logs", "AI Bridge")}</title>

			<BoundaryLogsPageView
				isLoading={boundaryLogsQuery.isLoading}
				logs={boundaryLogsQuery.data?.logs}
				boundaryLogsQuery={boundaryLogsQuery}
				filterProps={{
					filter,
					error: boundaryLogsQuery.error,
					menus: {
						user: userMenu,
						action: actionMenu,
						organization: showOrganizations ? organizationsMenu : undefined,
					},
				}}
			/>
		</>
	);
};

export default BoundaryLogsPage;
