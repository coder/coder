import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";

const ExternalAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();

	return (
		<>
			<Helmet>
				<title>{pageTitle("External Authentication Settings")}</title>
			</Helmet>
			<ExternalAuthSettingsPageView config={deploymentConfig.config} />
		</>
	);
};

export default ExternalAuthSettingsPage;
