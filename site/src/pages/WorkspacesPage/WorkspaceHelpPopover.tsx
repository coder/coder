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

export const WorkspaceHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger />
			<HelpPopoverContent>
				<HelpPopoverTitle>What is a workspace?</HelpPopoverTitle>
				<HelpPopoverText>
					A workspace is your development environment in the cloud. It includes
					the infrastructure and tools you need to work on your project.
				</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/user-guides")}>
						Create Workspaces
					</HelpPopoverLink>
					<HelpPopoverLink href={docs("/user-guides/workspace-access")}>
						Connect with SSH
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};
