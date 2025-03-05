import Link from "@mui/material/Link";
import type * as TypesGen from "api/typesGenerated";
import { TerminalIcon } from "components/Icons/TerminalIcon";
import type { FC, MouseEvent } from "react";
import { generateRandomString } from "utils/random";
import { AgentButton } from "../AgentButton";
import { DisplayAppNameMap } from "../AppLink/AppLink";

export const Language = {
	terminalTitle: (identifier: string): string => `Terminal - ${identifier}`,
};

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
	const params = new URLSearchParams();
	if (containerName) {
		params.append("container", containerName);
	}
	// Always use the primary for the terminal link. This is a relative link.
	const href = `/@${userName}/${workspaceName}${
		agentName ? `.${agentName}` : ""
	}/terminal?${params.toString()}`;

	return (
		<Link
			underline="none"
			color="inherit"
			component={AgentButton}
			startIcon={<TerminalIcon />}
			href={href}
			onClick={(event: MouseEvent<HTMLElement>) => {
				event.preventDefault();
				window.open(
					href,
					Language.terminalTitle(generateRandomString(12)),
					"width=900,height=600",
				);
			}}
			data-testid="terminal"
		>
			{DisplayAppNameMap.web_terminal}
		</Link>
	);
};
