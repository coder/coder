import type { FC } from "react";
import { pageTitle } from "utils/page";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";

const ExternalAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();

	return (
		<>
			<title>{pageTitle("External Authentication Settings")}</title>

			<ExternalAuthSettingsPageView config={deploymentConfig.config} />
		</>
	);
};

export default ExternalAuthSettingsPage;
