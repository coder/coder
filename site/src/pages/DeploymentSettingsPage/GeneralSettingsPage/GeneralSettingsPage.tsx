import { deploymentDAUs } from "api/queries/deployment";
import { availableExperiments, experiments } from "api/queries/experiments";
import { userStatusCountsOverTime } from "api/queries/insights";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { GeneralSettingsPageView } from "./GeneralSettingsPageView";

const GeneralSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();
	const deploymentDAUsQuery = useQuery(deploymentDAUs());
	const safeExperimentsQuery = useQuery(availableExperiments());

	const { metadata } = useEmbeddedMetadata();
	const enabledExperimentsQuery = useQuery(experiments(metadata.experiments));
	const userStatusCountsOverTimeQuery = useQuery(userStatusCountsOverTime());
	const safeExperiments = safeExperimentsQuery.data?.safe ?? [];
	const invalidExperiments =
		enabledExperimentsQuery.data?.filter((exp) => {
			return !safeExperiments.includes(exp);
		}) ?? [];

	return (
		<>
			<Helmet>
				<title>{pageTitle("General Settings")}</title>
			</Helmet>
			<GeneralSettingsPageView
				deploymentOptions={deploymentConfig.options}
				deploymentDAUs={deploymentDAUsQuery.data}
				deploymentDAUsError={deploymentDAUsQuery.error}
				invalidExperiments={invalidExperiments}
				safeExperiments={safeExperiments}
				userStatusCountsOverTime={userStatusCountsOverTimeQuery.data}
			/>
		</>
	);
};

export default GeneralSettingsPage;
