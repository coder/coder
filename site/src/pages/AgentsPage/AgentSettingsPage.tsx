import { useAuthenticated } from "hooks";
import type { FC } from "react";
import { useParams } from "react-router";
import { AgentPageHeader } from "./AgentPageHeader";
import { AgentSettingsPageView } from "./AgentSettingsPageView";

const AgentSettingsPage: FC = () => {
	const { section } = useParams();
	const { permissions } = useAuthenticated();
	const isAgentsAdmin = permissions.editDeploymentConfig;
	return (
		<>
			<AgentPageHeader />
			<AgentSettingsPageView
				activeSection={section ?? "behavior"}
				canManageChatModelConfigs={isAgentsAdmin}
				canSetSystemPrompt={isAgentsAdmin}
			/>
		</>
	);
};

export default AgentSettingsPage;
