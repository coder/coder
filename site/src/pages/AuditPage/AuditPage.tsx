import { paginatedAudits } from "api/queries/audits";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { isNonInitialPage } from "components/PaginationWidget/utils";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useOrganizationsFilterMenu } from "modules/tableFiltering/options";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useActionFilterMenu, useResourceTypeFilterMenu } from "./AuditFilter";
import { AuditPageView } from "./AuditPageView";

const AuditPage: FC = () => {
	const feats = useFeatureVisibility();
	// The "else false" is required if audit_log is undefined.
	// It may happen if owner removes the license.
	//
	// see: https://github.com/coder/coder/issues/14798
	const isAuditLogVisible = feats.audit_log || false;

	const { showOrganizations } = useDashboard();

	/**
	 * There is an implicit link between auditsQuery and filter via the
	 * searchParams object
	 *
	 * @todo Make link more explicit (probably by making it so that components
	 * and hooks can share the result of useSearchParams directly)
	 */
	const [searchParams, setSearchParams] = useSearchParams();
	const auditsQuery = usePaginatedQuery(paginatedAudits(searchParams));
	const filter = useFilter({
		searchParamsResult: [searchParams, setSearchParams],
		onUpdate: auditsQuery.goToFirstPage,
	});

	const userMenu = useUserFilterMenu({
		value: filter.values.username,
		onChange: (option) =>
			filter.update({
				...filter.values,
				username: option?.value,
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

	const resourceTypeMenu = useResourceTypeFilterMenu({
		value: filter.values.resource_type,
		onChange: (option) =>
			filter.update({
				...filter.values,
				resource_type: option?.value,
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

	if (auditsQuery.error) {
		return (
			<div className="p-6">
				<ErrorAlert error={auditsQuery.error} />
			</div>
		);
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle("Audit")}</title>
			</Helmet>

			<AuditPageView
				auditLogs={auditsQuery.data?.audit_logs}
				isNonInitialPage={isNonInitialPage(searchParams)}
				isAuditLogVisible={isAuditLogVisible}
				auditsQuery={auditsQuery}
				error={auditsQuery.error}
				showOrgDetails={showOrganizations}
				filterProps={{
					filter,
					error: auditsQuery.error,
					menus: {
						user: userMenu,
						action: actionMenu,
						resourceType: resourceTypeMenu,
						organization: showOrganizations ? organizationsMenu : undefined,
					},
				}}
			/>
		</>
	);
};

export default AuditPage;
