import type { FC } from "react";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";

export const BoundaryLogsHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger />
			<HelpTooltipContent>
				<HelpTooltipTitle>What are boundary logs?</HelpTooltipTitle>
				<HelpTooltipText>
					Boundary logs record resource access events from the Boundary network
					isolation tool running in your workspaces. Each entry shows whether a
					resource access request (such as a network call or file access) was
					allowed or denied based on your configured policies.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
