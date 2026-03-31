import type { FC } from "react";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIconTrigger,
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
} from "#/components/HelpTooltip/HelpTooltip";
import { docs } from "#/utils/docs";

export const ConnectionLogHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger />

			<HelpTooltipContent>
				<HelpTooltipTitle>{"Why are some events missing?"}</HelpTooltipTitle>
				<HelpTooltipText>
					{
						"The connection log is a best-effort log of workspace access. Some events are reported by workspace agents, and receipt of these events by the server is not guaranteed."
					}
				</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/admin/monitoring/connection-logs")}>
						{"Connection log documentation"}
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
