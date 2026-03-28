import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { LimitsTab } from "../components/LimitsTab";

const AgentSettingsLimitsPage: FC = () => {
	const { permissions } = useAuthenticated();
	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<LimitsTab />
		</RequirePermission>
	);
};

export default AgentSettingsLimitsPage;
