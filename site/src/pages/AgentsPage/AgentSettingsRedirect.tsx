import type { FC } from "react";
import { Navigate, useLocation, useParams } from "react-router";
import {
	DEFAULT_AGENT_SETTINGS_SECTION,
	isAgentSettingsSection,
	withAgentSettingsSection,
} from "./utils/agentSettingsSection";

/**
 * Sections that moved to the deployment-level AI settings pages, plus the
 * slugs they were previously reachable under.
 */
const AI_SETTINGS_REDIRECTS: Record<string, string> = {
	admin: "coder-agents",
	agents: "coder-agents",
	"coder-agents": "coder-agents",
	experiments: "coder-agents",
	instructions: "instructions",
	lifecycle: "lifecycle",
	"mcp-servers": "mcp-servers",
	models: "models",
	providers: "providers",
	templates: "templates",
};

/**
 * The personal agent settings live in a dialog over the chat routes, so the
 * former `/agents/settings/*` URLs redirect to the `settings` search param.
 */
const AgentSettingsRedirect: FC = () => {
	const { section } = useParams<{ section?: string }>();
	const location = useLocation();

	const aiSettingsSection = section
		? AI_SETTINGS_REDIRECTS[section]
		: undefined;
	if (aiSettingsSection) {
		return <Navigate to={`/ai/settings/${aiSettingsSection}`} replace />;
	}

	return (
		<Navigate
			to={{
				pathname: "/agents",
				search: withAgentSettingsSection(
					location.search,
					isAgentSettingsSection(section)
						? section
						: DEFAULT_AGENT_SETTINGS_SECTION,
				),
			}}
			replace
		/>
	);
};

export default AgentSettingsRedirect;
