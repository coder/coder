import { provisionerJobs } from "api/queries/organizations";
import type { ProvisionerJobStatus } from "api/typesGenerated";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import OrganizationProvisionerJobsPageView from "./OrganizationProvisionerJobsPageView";

const OrganizationProvisionerJobsPage: FC = () => {
	const { organization } = useOrganizationSettings();
	const [params, setParams] = useSearchParams();
	const filter = {
		status: params.get("status") || "",
	};
	const {
		data: jobs,
		isLoadingError,
		refetch,
	} = useQuery({
		...provisionerJobs(
			organization?.id || "",
			filter.status as ProvisionerJobStatus,
		),
		enabled: organization !== undefined,
	});

	return (
		<OrganizationProvisionerJobsPageView
			jobs={jobs}
			filter={filter}
			organization={organization}
			error={isLoadingError}
			onRetry={refetch}
			onFilterChange={setParams}
		/>
	);
};

export default OrganizationProvisionerJobsPage;
