import RefreshIcon from "@material-ui/icons/Refresh"
import { FC } from "react"
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip"

const Language = {
  outdatedLabel: "Outdated",
  versionTooltipText: "This workspace version is outdated and a newer version is available.",
  updateVersionLabel: "Update version",
}

interface TooltipProps {
  onUpdateVersion: () => void
}

export const OutdatedHelpTooltip: FC<TooltipProps> = ({ onUpdateVersion }) => {
  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{Language.outdatedLabel}</HelpTooltipTitle>
      <HelpTooltipText>{Language.versionTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipAction icon={RefreshIcon} onClick={onUpdateVersion}>
          {Language.updateVersionLabel}
        </HelpTooltipAction>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}
