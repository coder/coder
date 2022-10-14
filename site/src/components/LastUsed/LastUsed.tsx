import { makeStyles, Theme, useTheme } from "@material-ui/core/styles"
import { FC } from "react"

import Icon from "@material-ui/icons/Brightness1"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import { colors } from "theme/colors"

dayjs.extend(relativeTime)

interface LastUsedProps {
  lastUsedAt: string
}

export const LastUsed: FC<LastUsedProps> = ({ lastUsedAt }) => {
  const theme: Theme = useTheme()
  const styles = useStyles()

  const t = dayjs(lastUsedAt)
  const now = dayjs()

  let color = theme.palette.text.secondary
  let message = t.fromNow()
  let displayCircle = true

  if (t.isAfter(now.subtract(1, "hour"))) {
    color = colors.green[9]
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now"
  } else if (t.isAfter(now.subtract(3, "day"))) {
    color = theme.palette.text.secondary
  } else if (t.isAfter(now.subtract(1, "month"))) {
    color = theme.palette.warning.light
  } else if (t.isAfter(now.subtract(100, "year"))) {
    color = colors.red[10]
  } else {
    // color = theme.palette.error.light
    message = "Never"
    displayCircle = false
  }

  return (
    <span className={styles.root}>
      <span
        style={{
          color: color,
          display: displayCircle ? undefined : "none",
        }}
      >
        <Icon className={styles.icon} />
      </span>
      <span data-chromatic="ignore">{message}</span>
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
  },
  icon: {
    marginRight: 8,
    width: 10,
    height: 10,
  },
}))
