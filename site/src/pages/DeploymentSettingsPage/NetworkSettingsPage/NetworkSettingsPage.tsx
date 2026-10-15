import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { pageTitle } from "#/utils/page";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const NetworkSettingsPage: React.FC = () => {
	const { deploymentConfig } = useDeploymentConfig();

	return (
		<>
			<title>{pageTitle("Network settings")}</title>

			<NetworkSettingsPageView options={deploymentConfig.options} />
		</>
	);
};

export default NetworkSettingsPage;
