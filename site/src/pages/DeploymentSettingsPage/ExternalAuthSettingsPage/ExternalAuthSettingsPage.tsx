import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { pageTitle } from "#/utils/page";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";

const ExternalAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const { permissions } = useAuthenticated();
	const { multiple_external_auth: isEntitled } = useFeatureVisibility();

	return (
		<>
			<title>{pageTitle("External Authentication Settings")}</title>

			<ExternalAuthSettingsPageView
				config={deploymentConfig.config}
				isEntitled={isEntitled}
				canViewPremium={permissions.viewAllLicenses}
			/>
		</>
	);
};

export default ExternalAuthSettingsPage;
