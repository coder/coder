import { deploymentDAUs } from "api/queries/deployment";
import { entitlements } from "api/queries/entitlements";
import { availableExperiments, experiments } from "api/queries/experiments";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { GeneralSettingsPageView } from "./GeneralSettingsPageView";

const GeneralSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();
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
				<title>{pageTitle("General Settings")}</title>
			</Helmet>
			<GeneralSettingsPageView
				deploymentOptions={deploymentConfig.options}
				dailyActiveUsers={dailyActiveUsers}
				invalidExperiments={invalidExperiments}
				safeExperiments={safeExperiments}
			/>
		</>
	);
};

export default GeneralSettingsPage;
