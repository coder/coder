import { FC } from "react"
import { HelpTooltip, HelpTooltipText, HelpTooltipTitle } from "./HelpTooltip"

export const Language = {
  label: "Agent Outdated",
  text: "This agent is an older version than the Coder server. This can happen after you update Coder with running workspaces. To fix this, you can stop and start the workspace.",
}

interface TooltipProps {
  outdated: boolean
}

export const AgentOutdatedTooltip: FC<React.PropsWithChildren<TooltipProps>> = ({ outdated }) => {
  if (!outdated) {
    return null
  }
  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{Language.label}</HelpTooltipTitle>
      <HelpTooltipText>{Language.text}</HelpTooltipText>
    </HelpTooltip>
  )
}
