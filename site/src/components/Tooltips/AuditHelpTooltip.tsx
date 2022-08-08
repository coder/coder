import { FC } from "react"
import { HelpTooltip, HelpTooltipText, HelpTooltipTitle } from "./HelpTooltip"

const Language = {
  title: "What is an audit log?",
  body: "An audit log is a record of events and changes made throughout a system.",
}

export const AuditHelpTooltip: FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>{Language.title}</HelpTooltipTitle>
      <HelpTooltipText>{Language.body}</HelpTooltipText>
    </HelpTooltip>
  )
}
