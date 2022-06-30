import { FC } from "react"
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip"

const Language = {
  workspaceTooltipTitle: "What is a workspace?",
  workspaceTooltipText:
    "A workspace is your development environment in the cloud. It includes the infrastructure and tools you need to work on your project.",
  workspaceTooltipLink1: "Create workspaces",
  workspaceTooltipLink2: "Connect with SSH",
  workspaceTooltipLink3: "Editors and IDEs",
}

export const WorkspaceHelpTooltip: FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>{Language.workspaceTooltipTitle}</HelpTooltipTitle>
      <HelpTooltipText>{Language.workspaceTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#create-workspaces">
          {Language.workspaceTooltipLink1}
        </HelpTooltipLink>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#connect-with-ssh">
          {Language.workspaceTooltipLink2}
        </HelpTooltipLink>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#editors-and-ides">
          {Language.workspaceTooltipLink3}
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}
