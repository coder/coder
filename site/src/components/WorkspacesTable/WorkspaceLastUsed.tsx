import { Theme, useTheme } from "@material-ui/core/styles"
import { FC } from "react"

import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"

dayjs.extend(relativeTime)

interface WorkspaceLastUsedProps {
  lastUsedAt: string
}

export const WorkspaceLastUsed: FC<WorkspaceLastUsedProps> = ({ lastUsedAt }) => {
  const theme: Theme = useTheme()

  const t = dayjs(lastUsedAt)
  const now = dayjs()

  let color = theme.palette.text.secondary
  let message = t.fromNow()

  if (t.isAfter(now.subtract(1, "hour"))) {
    color = theme.palette.success.main
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Last Hour"
  } else if (t.isAfter(now.subtract(1, "day"))) {
    color = theme.palette.primary.main
  } else if (t.isAfter(now.subtract(1, "month"))) {
    color = theme.palette.text.secondary
  } else if (t.isAfter(now.subtract(100, "year"))) {
    color = theme.palette.warning.light
  } else {
    color = theme.palette.error.light
    message = "Never"
  }

  return <span style={{ color: color }}>{message}</span>
}
