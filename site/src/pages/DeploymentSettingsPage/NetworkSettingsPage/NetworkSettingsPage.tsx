import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";

const NetworkSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Network Settings")}</title>
			</Helmet>

			{deploymentConfig ? (
				<NetworkSettingsPageView options={deploymentConfig.options} />
			) : (
				<Loader />
			)}
		</>
	);
};

export default NetworkSettingsPage;
