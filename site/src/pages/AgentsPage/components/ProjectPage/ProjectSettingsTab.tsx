import type { ChatProject } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { AgentModelSection } from "./AgentModelSection";
import { AgentToolsSection } from "./AgentToolsSection";
import { AgentTriggersSection } from "./AgentTriggersSection";
import { AgentWorkspacesSection } from "./AgentWorkspacesSection";
import { ConnectedAccountsSection } from "./ConnectedAccountsSection";
import { InvokableAgentsSection } from "./InvokableAgentsSection";
import { ProjectGeneralSection } from "./ProjectGeneralSection";
import { ProjectInstructionsSection } from "./ProjectInstructionsSection";
import { SettingsGroup } from "./ProjectSection";

type ProjectSettingsTabProps = {
	readonly project: ChatProject;
};

/** The agent's configuration, in groups of labeled sections. */
export const ProjectSettingsTab: React.FC<ProjectSettingsTabProps> = ({
	project,
}) => (
	<div className="flex flex-col gap-6">
		{!project.permissions.edit && (
			<Alert severity="info">
				You can view these settings. Only the owner and people with edit access
				can change them.
			</Alert>
		)}
		<div className="flex flex-col gap-8">
			<SettingsGroup
				title="Behavior"
				description="What this agent is told to do, and what starts its chats."
			>
				<ProjectInstructionsSection project={project} />
				<AgentModelSection project={project} />
				<AgentTriggersSection project={project} />
			</SettingsGroup>
			<SettingsGroup
				title="Capabilities"
				description="What chats in this agent can use and do."
			>
				<AgentToolsSection project={project} />
				<InvokableAgentsSection project={project} />
				<AgentWorkspacesSection project={project} />
				<ConnectedAccountsSection project={project} />
			</SettingsGroup>
			<ProjectGeneralSection project={project} />
		</div>
	</div>
);
