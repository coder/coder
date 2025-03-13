import { provisionerJobs } from "api/queries/organizations";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import OrganizationProvisionerJobsPageView from "./OrganizationProvisionerJobsPageView";

const OrganizationProvisionerJobsPage: FC = () => {
	const { organization } = useOrganizationSettings();
	const {
		data: jobs,
		isLoadingError,
		refetch,
	} = useQuery({
		...provisionerJobs(organization?.id || ""),
		enabled: organization !== undefined,
	});

	return (
		<OrganizationProvisionerJobsPageView
			jobs={jobs}
			organization={organization}
			error={isLoadingError}
			onRetry={refetch}
		/>
	);
};

export default OrganizationProvisionerJobsPage;
