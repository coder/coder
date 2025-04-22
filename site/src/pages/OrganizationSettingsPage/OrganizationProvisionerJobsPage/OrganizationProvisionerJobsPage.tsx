import type { GetProvisionerJobsParams } from "api/api";
import { provisionerJobs } from "api/queries/organizations";
import type { ProvisionerJobStatus } from "api/typesGenerated";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import OrganizationProvisionerJobsPageView from "./OrganizationProvisionerJobsPageView";

const OrganizationProvisionerJobsPage: FC = () => {
	const { organization } = useOrganizationSettings();
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = {
		status: searchParams.get("status") || "",
	};
	const queryParams = {
		...filter,
		limit: 100,
	} as GetProvisionerJobsParams;
	const {
		data: jobs,
		isLoadingError,
		refetch,
	} = useQuery({
		...provisionerJobs(organization?.id || "", queryParams),
		enabled: organization !== undefined,
	});

	return (
		<OrganizationProvisionerJobsPageView
			jobs={jobs}
			filter={filter}
			organization={organization}
			error={isLoadingError}
			onRetry={refetch}
			onFilterChange={setSearchParams}
		/>
	);
};

export default OrganizationProvisionerJobsPage;
