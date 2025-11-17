import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { pageTitle } from "utils/page";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const ObservabilitySettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const { entitlements } = useDashboard();
	const { multiple_organizations: hasPremiumLicense, aibridge: hasAIBridgeEnabled } = useFeatureVisibility();

	return (
		<>
			<title>{pageTitle("Observability Settings")}</title>

			<ObservabilitySettingsPageView
				options={deploymentConfig.options}
				featureAuditLogEnabled={entitlements.features.audit_log.enabled}
				isPremium={hasPremiumLicense}
				isAIBridgeEnabled={hasAIBridgeEnabled}
			/>
		</>
	);
};

export default ObservabilitySettingsPage;
