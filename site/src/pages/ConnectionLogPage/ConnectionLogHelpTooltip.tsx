import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIconTrigger,
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import type { FC } from "react";
import { docs } from "utils/docs";

const Language = {
	title: "Why are some events missing?",
	body: "The connection log is a best-effort log of workspace access. Some events are reported by workspace agents, and receipt of these events by the server is not guaranteed.",
	docs: "Connection log documentation",
};

export const ConnectionLogHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger />

			<HelpTooltipContent>
				<HelpTooltipTitle>{Language.title}</HelpTooltipTitle>
				<HelpTooltipText>{Language.body}</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/admin/monitoring/connection-logs")}>
						{Language.docs}
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
