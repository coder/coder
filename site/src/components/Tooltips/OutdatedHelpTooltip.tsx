import RefreshIcon from "@material-ui/icons/Refresh"
import { FC } from "react"
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip"
import InfoIcon from "@material-ui/icons/InfoOutlined"
import { makeStyles } from "@material-ui/core/styles"
import { colors } from "theme/colors"

export const Language = {
  outdatedLabel: "Outdated",
  versionTooltipText:
    "This workspace version is outdated and a newer version is available.",
  updateVersionLabel: "Update version",
}

interface TooltipProps {
  onUpdateVersion: () => void
  ariaLabel?: string
}

export const OutdatedHelpTooltip: FC<React.PropsWithChildren<TooltipProps>> = ({
  onUpdateVersion,
  ariaLabel,
}) => {
  const styles = useStyles()

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={styles.icon}
      buttonClassName={styles.button}
    >
      <HelpTooltipTitle>{Language.outdatedLabel}</HelpTooltipTitle>
      <HelpTooltipText>{Language.versionTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipAction
          icon={RefreshIcon}
          onClick={onUpdateVersion}
          ariaLabel={ariaLabel}
        >
          {Language.updateVersionLabel}
        </HelpTooltipAction>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}

const useStyles = makeStyles(() => ({
  icon: {
    color: colors.yellow[5],
  },

  button: {
    opacity: 1,

    "&:hover": {
      opacity: 1,
    },
  },
}))
