import { deploymentDAUs } from "api/queries/deployment";
import { availableExperiments } from "api/queries/experiments";
import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { OverviewPageView } from "./OverviewPageView";

const OverviewPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const safeExperimentsQuery = useQuery(availableExperiments());

	const safeExperiments = safeExperimentsQuery.data?.safe ?? [];

	const { data: dailyActiveUsers } = useQuery(deploymentDAUs());

	return (
		<>
			<title>{pageTitle("Overview", "Deployment")}</title>

			<OverviewPageView
				deploymentOptions={deploymentConfig.options}
				dailyActiveUsers={dailyActiveUsers}
				safeExperiments={safeExperiments}
			/>
		</>
	);
};

export default OverviewPage;
