import { HelpTooltip, HelpTooltipText, HelpTooltipTitle } from "./HelpTooltip/HelpTooltip"

export const Language = {
  agentTooltipTitle: "What is an agent?",
  agentTooltipText:
    "The Coder agent runs inside your resource and gives you direct access to the shell via the UI or CLI.",
}

export const AgentHelpTooltip: React.FC<React.PropsWithChildren<unknown>> = () => {
  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{Language.agentTooltipTitle}</HelpTooltipTitle>
      <HelpTooltipText>{Language.agentTooltipText}</HelpTooltipText>
    </HelpTooltip>
  )
}
