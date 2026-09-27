import type { ComponentProps, FC } from "react";
import { Link, useLocation } from "react-router";
import {
	type AgentSettingsSection,
	DEFAULT_AGENT_SETTINGS_SECTION,
	withAgentSettingsSection,
} from "../../utils/agentSettingsSection";

type AgentSettingsLinkProps = Omit<ComponentProps<typeof Link>, "to"> & {
	section?: AgentSettingsSection;
};

/**
 * Opens the agent settings dialog over the current route, preserving the
 * pathname and any sidebar filter params already in the URL.
 */
export const AgentSettingsLink: FC<AgentSettingsLinkProps> = ({
	section = DEFAULT_AGENT_SETTINGS_SECTION,
	...linkProps
}) => {
	const location = useLocation();

	return (
		<Link
			to={{
				pathname: location.pathname,
				search: withAgentSettingsSection(location.search, section),
			}}
			{...linkProps}
		/>
	);
};
