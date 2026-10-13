import { GlobeIcon } from "lucide-react";
import { getBrowserHref, openAppInNewWindow } from "#/modules/apps/apps";
import { AgentButton } from "../AgentButton";

type BrowserLinkProps = {
	workspaceName: string;
	agentName?: string;
	userName?: string;
};

/** Opens the built-in workspace browser for the given agent. */
export const BrowserLink: React.FC<BrowserLinkProps> = ({
	agentName,
	userName = "me",
	workspaceName,
}) => {
	const href = getBrowserHref({
		username: userName,
		workspace: workspaceName,
		agent: agentName,
	});

	return (
		<AgentButton asChild>
			<a
				href={href}
				onClick={(event: React.MouseEvent<HTMLElement>) => {
					event.preventDefault();
					openAppInNewWindow(href);
				}}
			>
				<GlobeIcon />
				Browser
			</a>
		</AgentButton>
	);
};
