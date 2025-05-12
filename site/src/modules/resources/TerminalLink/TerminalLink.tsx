import { TerminalIcon } from "components/Icons/TerminalIcon";
import { getTerminalHref, openAppInNewWindow } from "modules/apps/apps";
import type { FC, MouseEvent } from "react";
import { AgentButton } from "../AgentButton";
import { DisplayAppNameMap } from "../AppLink/AppLink";

export interface TerminalLinkProps {
	workspaceName: string;
	agentName?: string;
	userName?: string;
	containerName?: string;
}

/**
 * Generate a link to a terminal connected to the provided workspace agent.  If
 * no agent is provided connect to the first agent.
 *
 * If no user name is provided "me" is used however it makes the link not
 * shareable.
 */
export const TerminalLink: FC<TerminalLinkProps> = ({
	agentName,
	userName = "me",
	workspaceName,
	containerName,
}) => {
	const href = getTerminalHref({
		username: userName,
		workspace: workspaceName,
		agent: agentName,
		container: containerName,
	});

	return (
		<AgentButton asChild>
			<a
				href={href}
				onClick={(event: MouseEvent<HTMLElement>) => {
					event.preventDefault();
					openAppInNewWindow(href);
				}}
			>
				<TerminalIcon />
				{DisplayAppNameMap.web_terminal}
			</a>
		</AgentButton>
	);
};
