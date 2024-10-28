import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";

const ExternalAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();

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
