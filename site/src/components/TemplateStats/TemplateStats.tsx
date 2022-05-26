import { makeStyles } from "@material-ui/core/styles"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import { Template, TemplateVersion } from "../../api/typesGenerated"
import { CardRadius, MONOSPACE_FONT_FAMILY } from "../../theme/constants"

dayjs.extend(relativeTime)

const Language = {
  usedByLabel: "Used by",
  activeVersionLabel: "Active version",
  lastUpdateLabel: "Last updated",
  userPlural: "users",
  userSingular: "user",
}

export interface TemplateStatsProps {
  template: Template
  activeVersion: TemplateVersion
}

export const TemplateStats: React.FC<TemplateStatsProps> = ({ template, activeVersion }) => {
  const styles = useStyles()

  return (
    <div className={styles.stats}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.usedByLabel}</span>

        <span className={styles.statsValue}>
          {template.workspace_owner_count}{" "}
          {template.workspace_owner_count === 1 ? Language.userSingular : Language.userPlural}
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.activeVersionLabel}</span>
        <span className={styles.statsValue}>{activeVersion.name}</span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.lastUpdateLabel}</span>
        <span className={styles.statsValue} data-chromatic="ignore">
          {dayjs().to(dayjs(template.updated_at))}
        </span>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    backgroundColor: theme.palette.background.paper,
    borderRadius: CardRadius,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    border: `1px solid ${theme.palette.divider}`,
  },

  statItem: {
    minWidth: theme.spacing(20),
    padding: theme.spacing(2),
    paddingTop: theme.spacing(1.75),
  },

  statsLabel: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
  },

  statsValue: {
    fontSize: 16,
    marginTop: theme.spacing(0.25),
    display: "inline-block",
  },

  statsDivider: {
    width: 1,
    height: theme.spacing(5),
    backgroundColor: theme.palette.divider,
    marginRight: theme.spacing(2),
  },
}))
