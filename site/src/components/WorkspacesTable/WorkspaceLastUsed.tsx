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

  if (t.isBefore(now.subtract(100, "year"))) {
    color = theme.palette.error.main
    message = "Never"
  } else if (t.isBefore(now.subtract(1, "month"))) {
    color = theme.palette.warning.light
  } else if (t.isAfter(now.subtract(24, "hour"))) {
    // Since the agent reports on a regular interval,
    // we default to "Today" instead of showing a
    // potentially inaccurate value.
    color = theme.palette.success.main
    message = "Today"
  }

  return <span style={{ color: color }}>{message}</span>
}
