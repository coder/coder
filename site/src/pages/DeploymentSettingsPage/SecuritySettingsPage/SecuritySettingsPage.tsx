import { useDashboard } from "modules/dashboard/useDashboard";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";

const SecuritySettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();
	const { entitlements } = useDashboard();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Security Settings")}</title>
			</Helmet>
			<SecuritySettingsPageView
				options={deploymentConfig.options}
				featureBrowserOnlyEnabled={entitlements.features.browser_only.enabled}
			/>
		</>
	);
};

export default SecuritySettingsPage;
