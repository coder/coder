import { useSearchParams } from "react-router";
import { paginatedAudits } from "#/api/queries/audits";
import { useFilter } from "#/components/Filter/Filter";
import { isNonInitialPage } from "#/components/PaginationWidget/utils";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { pageTitle } from "#/utils/page";
import { AuditPageView } from "./AuditPageView";

const AuditPage: React.FC = () => {
	const { permissions } = useAuthenticated();
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
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: auditsQuery.goToFirstPage,
	});

	return (
		<>
			<title>{pageTitle("Audit")}</title>

			<AuditPageView
				auditLogs={auditsQuery.data?.audit_logs}
				isNonInitialPage={isNonInitialPage(searchParams)}
				isAuditLogVisible={isAuditLogVisible}
				auditsQuery={auditsQuery}
				error={auditsQuery.error}
				showOrgDetails={showOrganizations}
				permissions={permissions}
				filterProps={{
					filter,
					error: auditsQuery.error,
				}}
			/>
		</>
	);
};

export default AuditPage;
