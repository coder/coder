import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AdminBadge } from "./components/AdminBadge";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel/ChatModelAdminPanel";

const AgentSettingsProvidersPage: FC = () => {
	const { permissions } = useAuthenticated();

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<ChatModelAdminPanel
				section="providers"
				sectionLabel="Providers"
				sectionDescription="Connect third-party LLM services like OpenAI, Anthropic, or Google. Each provider supplies models that users can select for their chats."
				sectionBadge={<AdminBadge />}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsProvidersPage;
