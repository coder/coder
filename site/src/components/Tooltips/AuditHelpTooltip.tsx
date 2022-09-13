import { FC } from "react"
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip"

export const Language = {
  title: "What is an audit log?",
  body: "An audit log is a record of events and changes made throughout a system.",
  docs: "Events we track",
}

export const AuditHelpTooltip: FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>{Language.title}</HelpTooltipTitle>
      <HelpTooltipText>{Language.body}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/admin/audit-logs">
          {Language.docs}
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}
