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

export const AIBridgeHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger />

			<HelpTooltipContent>
				<HelpTooltipTitle>What is AI Bridge?</HelpTooltipTitle>
				<HelpTooltipText>
					AI Bridge is a smart gateway for AI that provides centralized
					management, auditing, and attribution for LLM usage.
				</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/ai-coder/ai-bridge")}>
						Read the docs
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
