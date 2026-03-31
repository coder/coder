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

const Language = {
	workspaceTooltipTitle: "What is a workspace?",
	workspaceTooltipText:
		"A workspace is your development environment in the cloud. It includes the infrastructure and tools you need to work on your project.",
	workspaceTooltipLink1: "Create Workspaces",
	workspaceTooltipLink2: "Connect with SSH",
	workspaceTooltipLink3: "Editors and IDEs",
};

export const WorkspaceHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger />
			<HelpPopoverContent>
				<HelpPopoverTitle>{Language.workspaceTooltipTitle}</HelpPopoverTitle>
				<HelpPopoverText>{Language.workspaceTooltipText}</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/user-guides")}>
						{Language.workspaceTooltipLink1}
					</HelpPopoverLink>
					<HelpPopoverLink href={docs("/user-guides/workspace-access")}>
						{Language.workspaceTooltipLink2}
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
