import type { FC } from "react";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverLink,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { docs } from "#/utils/docs";

export const AIBridgeHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger />

			<HelpPopoverContent>
				<HelpPopoverTitle>What is AI Bridge?</HelpPopoverTitle>
				<HelpPopoverText>
					AI Bridge is a smart gateway for AI that provides centralized
					management, auditing, and attribution for LLM usage.
				</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/ai-coder/ai-bridge")}>
						Read the docs
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
