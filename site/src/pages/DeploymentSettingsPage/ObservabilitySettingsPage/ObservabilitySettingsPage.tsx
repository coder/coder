import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { pageTitle } from "#/utils/page";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const ObservabilitySettingsPage: React.FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const { entitlements } = useDashboard();
	const { permissions } = useAuthenticated();

	return (
		<>
			<title>{pageTitle("Observability Settings")}</title>

			<ObservabilitySettingsPageView
				options={deploymentConfig.options}
				featureAuditLogEnabled={entitlements.features.audit_log.enabled}
				canViewPremium={permissions.viewAllLicenses}
			/>
		</>
	);
};

export default ObservabilitySettingsPage;
