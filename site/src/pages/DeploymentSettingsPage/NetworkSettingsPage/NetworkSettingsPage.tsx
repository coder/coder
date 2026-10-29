import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { pageTitle } from "#/utils/page";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const NetworkSettingsPage: React.FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const { entitlements } = useDashboard();
	const { permissions } = useAuthenticated();

	return (
		<>
			<title>{pageTitle("Network Settings")}</title>

			<NetworkSettingsPageView
				options={deploymentConfig.options}
				hasLicense={entitlements.has_license}
				canViewPremium={permissions.viewAllLicenses}
			/>
		</>
	);
};

export default NetworkSettingsPage;
