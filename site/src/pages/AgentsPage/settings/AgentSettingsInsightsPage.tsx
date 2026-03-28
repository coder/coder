import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { InsightsContent } from "../components/InsightsContent";

const AgentSettingsInsightsPage: FC = () => {
	const { permissions } = useAuthenticated();
	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<InsightsContent />
		</RequirePermission>
	);
};

export default AgentSettingsInsightsPage;
