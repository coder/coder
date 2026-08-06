import type { FC } from "react";
import { useSearchParams } from "react-router";
import { paginatedProvisionerJobs } from "#/api/queries/organizations";
import { isNonInitialPage } from "#/components/PaginationWidget/utils";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import OrganizationProvisionerJobsPageView from "./OrganizationProvisionerJobsPageView";

const OrganizationProvisionerJobsPage: FC = () => {
	const { organization } = useOrganizationSettings();
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = {
		status: searchParams.get("status") ?? "",
		ids: searchParams.get("ids") ?? "",
	};
	const jobsQuery = usePaginatedQuery(
		paginatedProvisionerJobs(organization?.id ?? "", searchParams),
	);

	return (
		<OrganizationProvisionerJobsPageView
			jobs={jobsQuery.data?.jobs}
			jobsQuery={jobsQuery}
			filter={filter}
			organization={organization}
			error={jobsQuery.error}
			isNonInitialPage={isNonInitialPage(searchParams)}
			onRetry={jobsQuery.refetch}
			onFilterChange={(nextFilter) => {
				setSearchParams(nextFilter);
			}}
		/>
	);
};

export default OrganizationProvisionerJobsPage;
