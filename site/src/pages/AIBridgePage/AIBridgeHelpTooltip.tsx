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
					AI Bridge is a proxy that unifies and audits LLM usage across your
					organization.
				</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/ai-coder/ai-bridge")}>
						What we track
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
