import type { FC } from "react";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { pageTitle } from "#/utils/page";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";

const SecuritySettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const { entitlements } = useDashboard();

	return (
		<>
			<title>{pageTitle("Security Settings")}</title>

			<SecuritySettingsPageView
				options={deploymentConfig.options}
				isBrowserOnlyEntitled={
					entitlements.features.browser_only.entitlement !== "not_entitled"
				}
				featureBrowserOnlyEnabled={entitlements.features.browser_only.enabled}
			/>
		</>
	);
};

export default SecuritySettingsPage;
