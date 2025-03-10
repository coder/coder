import { deploymentDAUs } from "api/queries/deployment";
import { availableExperiments, experiments } from "api/queries/experiments";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { OverviewPageView } from "./OverviewPageView";

const OverviewPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const safeExperimentsQuery = useQuery(availableExperiments());

	const { metadata } = useEmbeddedMetadata();
	const enabledExperimentsQuery = useQuery(experiments(metadata.experiments));

	const safeExperiments = safeExperimentsQuery.data?.safe ?? [];
	const invalidExperiments =
		enabledExperimentsQuery.data?.filter((exp) => {
			return !safeExperiments.includes(exp);
		}) ?? [];

	const { data: dailyActiveUsers } = useQuery(deploymentDAUs());

	return (
		<>
			<Helmet>
				<title>{pageTitle("Overview", "Deployment")}</title>
			</Helmet>
			<OverviewPageView
				deploymentOptions={deploymentConfig.options}
				dailyActiveUsers={dailyActiveUsers}
				invalidExperiments={invalidExperiments}
				safeExperiments={safeExperiments}
			/>
		</>
	);
};

export default OverviewPage;
