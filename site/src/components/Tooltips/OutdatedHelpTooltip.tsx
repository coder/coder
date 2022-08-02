import RefreshIcon from "@material-ui/icons/Refresh"
import { FC } from "react"
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip"

export const Language = {
  outdatedLabel: "Outdated",
  versionTooltipText: "This workspace version is outdated and a newer version is available.",
  updateVersionLabel: "Update version",
}

interface TooltipProps {
  onUpdateVersion: () => void
  ariaLabel?: string
}

export const OutdatedHelpTooltip: FC<React.PropsWithChildren<TooltipProps>> = ({ onUpdateVersion, ariaLabel }) => {
  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{Language.outdatedLabel}</HelpTooltipTitle>
      <HelpTooltipText>{Language.versionTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipAction icon={RefreshIcon} onClick={onUpdateVersion} ariaLabel={ariaLabel}>
          {Language.updateVersionLabel}
        </HelpTooltipAction>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}
