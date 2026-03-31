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

export const WorkspaceHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger />
			<HelpTooltipContent>
				<HelpTooltipTitle>What is a workspace?</HelpTooltipTitle>
				<HelpTooltipText>
					A workspace is your development environment in the cloud. It includes
					the infrastructure and tools you need to work on your project.
				</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/user-guides")}>
						Create Workspaces
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/workspace-access")}>
						Connect with SSH
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
