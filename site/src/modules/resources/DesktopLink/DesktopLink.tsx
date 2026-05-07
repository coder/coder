import { MonitorIcon } from "lucide-react";
import type { FC, MouseEvent } from "react";
import { getDesktopHref, openAppInNewWindow } from "#/modules/apps/apps";
import { AgentButton } from "../AgentButton";
import { DisplayAppNameMap } from "../AppLink/AppLink";

interface DesktopLinkProps {
	workspaceName: string;
	agentName?: string;
	userName?: string;
}

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
				{DisplayAppNameMap.desktop}
			</a>
		</AgentButton>
	);
};
