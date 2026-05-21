import type { FC } from "react";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { pageTitle } from "#/utils/page";
import { AIGovernanceSettingsPageView } from "./AIGovernanceSettingsPageView";

const AIGovernanceSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const { entitlements } = useDashboard();
	const isAiBridgeEntitled =
		entitlements.features.aibridge.entitlement === "entitled" ||
		entitlements.features.aibridge.entitlement === "grace_period";
	const isAIBridgeEnabled = entitlements.features.aibridge.enabled;

	return (
		<>
			<title>{pageTitle("AI Governance Settings")}</title>

			<AIGovernanceSettingsPageView
				options={deploymentConfig.options}
				featureAIBridgeEntitled={isAiBridgeEntitled}
				featureAIBridgeEnabled={isAIBridgeEnabled}
			/>
		</>
	);
};

export default AIGovernanceSettingsPage;
