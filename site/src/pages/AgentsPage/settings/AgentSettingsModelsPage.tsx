import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AdminBadge } from "../components/AdminBadge";
import { ChatModelAdminPanel } from "../components/ChatModelAdminPanel/ChatModelAdminPanel";

const AgentSettingsModelsPage: FC = () => {
	const { permissions } = useAuthenticated();
	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<ChatModelAdminPanel
				section="models"
				sectionLabel="Models"
				sectionDescription="Choose which models from your configured providers are available for users to select. You can set a default and adjust context limits."
				sectionBadge={<AdminBadge />}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsModelsPage;
