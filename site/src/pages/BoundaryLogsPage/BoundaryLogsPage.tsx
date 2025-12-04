import { paginatedBoundaryAuditLogs } from "api/queries/boundaryauditlog";
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
import { useDecisionFilterMenu } from "./BoundaryLogsFilter";
import { BoundaryLogsPageView } from "./BoundaryLogsPageView";

const BoundaryLogsPage: FC = () => {
	const feats = useFeatureVisibility();
	// Boundary logs require audit_log feature visibility
	const isBoundaryLogsVisible = feats.audit_log || false;

	const { showOrganizations } = useDashboard();

	const [searchParams, setSearchParams] = useSearchParams();
	const boundaryLogsQuery = usePaginatedQuery(
		paginatedBoundaryAuditLogs(searchParams),
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

	const decisionMenu = useDecisionFilterMenu({
		value: filter.values.decision,
		onChange: (option) =>
			filter.update({
				...filter.values,
				decision: option?.value,
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
			<title>{pageTitle("Boundary Logs")}</title>

			<BoundaryLogsPageView
				logs={boundaryLogsQuery.data?.logs}
				isNonInitialPage={isNonInitialPage(searchParams)}
				isBoundaryLogsVisible={isBoundaryLogsVisible}
				boundaryLogsQuery={boundaryLogsQuery}
				error={boundaryLogsQuery.error}
				showOrgDetails={showOrganizations}
				filterProps={{
					filter,
					error: boundaryLogsQuery.error,
					menus: {
						user: userMenu,
						decision: decisionMenu,
						organization: showOrganizations ? organizationsMenu : undefined,
					},
				}}
			/>
		</>
	);
};

export default BoundaryLogsPage;
