import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { createDayString } from "util/createDayString"
import {
  formatTemplateBuildTime,
  formatTemplateActiveDevelopers,
} from "util/templates"
import { Template, TemplateVersion } from "../../api/typesGenerated"

const Language = {
  usedByLabel: "Used by",
  buildTimeLabel: "Build time",
  activeVersionLabel: "Active version",
  lastUpdateLabel: "Last updated",
  developerPlural: "developers",
  developerSingular: "developer",
  createdByLabel: "Created by",
}

export interface TemplateStatsProps {
  template: Template
  activeVersion: TemplateVersion
}

export const TemplateStats: FC<TemplateStatsProps> = ({
  template,
  activeVersion,
}) => {
  const styles = useStyles()

  return (
    <div className={styles.stats}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.usedByLabel}:</span>

        <span className={styles.statsValue}>
          {formatTemplateActiveDevelopers(template.active_user_count)}{" "}
          {template.active_user_count === 1
            ? Language.developerSingular
            : Language.developerPlural}
        </span>
      </div>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.buildTimeLabel}:</span>

        <span className={styles.statsValue}>
          {formatTemplateBuildTime(template.build_time_stats.start_ms)}{" "}
        </span>
      </div>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>
          {Language.activeVersionLabel}:
        </span>
        <span className={styles.statsValue}>{activeVersion.name}</span>
      </div>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.lastUpdateLabel}:</span>
        <span className={styles.statsValue} data-chromatic="ignore">
          {createDayString(template.updated_at)}
        </span>
      </div>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.createdByLabel}:</span>
        <span className={styles.statsValue}>{template.created_by_name}</span>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    border: `1px solid ${theme.palette.divider}`,
    [theme.breakpoints.down("sm")]: {
      display: "block",
    },
  },

  statItem: {
    padding: theme.spacing(2),
    paddingTop: theme.spacing(1.75),
    display: "flex",
    alignItems: "baseline",
    gap: theme.spacing(1),
  },

  statsLabel: {
    display: "block",
    wordWrap: "break-word",
  },

  statsValue: {
    display: "block",
    wordWrap: "break-word",
    color: theme.palette.text.primary,
  },
}))
