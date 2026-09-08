import { MonitorIcon } from "lucide-react";
import type { FC, MouseEvent } from "react";
import { getDesktopHref, openAppInNewWindow } from "#/modules/apps/apps";
import { AgentButton } from "../AgentButton";

interface DesktopLinkProps {
	workspaceName: string;
	agentName?: string;
	userName?: string;
}

/**
 * Generate a link to the built-in virtual desktop of the provided workspace
 * agent. If no agent is provided connect to the first agent.
 */
export const DesktopLink: FC<DesktopLinkProps> = ({
	agentName,
	userName = "me",
	workspaceName,
}) => {
	const href = getDesktopHref({
		username: userName,
		workspace: workspaceName,
		agent: agentName,
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
				<MonitorIcon />
				Desktop
			</a>
		</AgentButton>
	);
};
