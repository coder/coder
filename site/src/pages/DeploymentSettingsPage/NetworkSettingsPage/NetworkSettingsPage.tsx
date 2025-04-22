import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const NetworkSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Network Settings")}</title>
			</Helmet>
			<NetworkSettingsPageView options={deploymentConfig.options} />
		</>
	);
};

export default NetworkSettingsPage;
