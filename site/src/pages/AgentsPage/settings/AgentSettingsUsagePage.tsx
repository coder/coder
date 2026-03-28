import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsUsagePageView } from "./AgentSettingsUsagePageView";

const AgentSettingsUsagePage: FC = () => {
	const { permissions } = useAuthenticated();
	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsUsagePageView />
		</RequirePermission>
	);
};

export default AgentSettingsUsagePage;
