import { FC } from "react"
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip"
import { docs } from "utils/docs"

const Language = {
  workspaceTooltipTitle: "What is a workspace?",
  workspaceTooltipText:
    "A workspace is your development environment in the cloud. It includes the infrastructure and tools you need to work on your project.",
  workspaceTooltipLink1: "Create Workspaces",
  workspaceTooltipLink2: "Connect with SSH",
  workspaceTooltipLink3: "Editors and IDEs",
}

export const WorkspaceHelpTooltip: FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>{Language.workspaceTooltipTitle}</HelpTooltipTitle>
      <HelpTooltipText>{Language.workspaceTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href={docs("/workspaces#create-workspaces")}>
          {Language.workspaceTooltipLink1}
        </HelpTooltipLink>
        <HelpTooltipLink href={docs("/workspaces#connect-with-ssh")}>
          {Language.workspaceTooltipLink2}
        </HelpTooltipLink>
        <HelpTooltipLink href={docs("/workspaces#editors-and-ides")}>
          {Language.workspaceTooltipLink3}
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}
