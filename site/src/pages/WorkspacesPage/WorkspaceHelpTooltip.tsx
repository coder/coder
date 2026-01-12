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
	workspaceTooltipTitle: "What is a workspace?",
	workspaceTooltipText:
		"A workspace is your development environment in the cloud. It includes the infrastructure and tools you need to work on your project.",
	workspaceTooltipLink1: "Create workspaces",
	workspaceTooltipLink2: "Connect with SSH",
	workspaceTooltipLink3: "Editors and IDEs",
};

export const WorkspaceHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger />
			<HelpTooltipContent>
				<HelpTooltipTitle>{Language.workspaceTooltipTitle}</HelpTooltipTitle>
				<HelpTooltipText>{Language.workspaceTooltipText}</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/user-guides")}>
						{Language.workspaceTooltipLink1}
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/workspace-access")}>
						{Language.workspaceTooltipLink2}
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
