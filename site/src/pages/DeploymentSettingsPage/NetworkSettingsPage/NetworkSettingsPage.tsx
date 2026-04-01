import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { pageTitle } from "utils/page";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const NetworkSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();

	return (
		<>
			<title>{pageTitle("Network Settings")}</title>

			<NetworkSettingsPageView options={deploymentConfig.options} />
		</>
	);
};

export default NetworkSettingsPage;
