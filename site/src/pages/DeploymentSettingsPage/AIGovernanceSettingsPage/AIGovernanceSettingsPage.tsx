import { useDashboard } from "modules/dashboard/useDashboard";
import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { pageTitle } from "utils/page";
import { AIGovernanceSettingsPageView } from "./AIGovernanceSettingsPageView";

const AIGovernanceSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();
	const { entitlements } = useDashboard();

	return (
		<>
			<title>{pageTitle("AI Governance Settings")}</title>

			<AIGovernanceSettingsPageView
				options={deploymentConfig.options}
				featureAIBridgeEntitled={
					entitlements.features.aibridge.entitlement === "entitled" ||
					entitlements.features.aibridge.entitlement === "grace_period"
				}
			/>
		</>
	);
};

export default AIGovernanceSettingsPage;
