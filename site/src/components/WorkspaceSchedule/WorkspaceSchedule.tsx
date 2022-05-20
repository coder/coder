import Box from "@material-ui/core/Box"
import Typography from "@material-ui/core/Typography"
import cronstrue from "cronstrue"
import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { extractTimezone, stripTimezone } from "../../util/schedule"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"

dayjs.extend(duration)
dayjs.extend(relativeTime)

const Language = {
  autoStartLabel: (schedule: string): string => {
    const prefix = "Start"

    if (schedule) {
      return `${prefix} (${extractTimezone(schedule)})`
    } else {
      return prefix
    }
  },
  autoStartDisplay: (schedule: string): string => {
    if (schedule) {
      return cronstrue.toString(stripTimezone(schedule), { throwExceptionOnParseError: false })
    }
    return "Manual"
  },
  autoStopLabel: "Shutdown",
  autoStopDisplay: (workspace: TypesGen.Workspace): string => {
    const latest = workspace.latest_build

    if (!workspace.ttl || workspace.ttl < 1) {
      return "Manual"
    }

    if (latest.transition === "start") {
      const now = dayjs()
      const updatedAt = dayjs(latest.updated_at)
      const deadline = updatedAt.add(workspace.ttl / 1_000_000, "ms")
      if (now.isAfter(deadline)) {
        return "workspace is shutting down now"
      }
      return now.to(deadline)
    }

    const duration = dayjs.duration(workspace.ttl / 1_000_000, "milliseconds")
    return `${duration.humanize()} after start`
  },
}

export interface WorkspaceScheduleProps {
  workspace: TypesGen.Workspace
}

/**
 * WorkspaceSchedule displays a workspace schedule in a human-readable format
 *
 * @remarks Visual Component
 */
export const WorkspaceSchedule: React.FC<WorkspaceScheduleProps> = ({ workspace }) => {
  return (
    <WorkspaceSection title="Workspace schedule">
      <Box mt={2}>
        <Typography variant="h6">{Language.autoStartLabel(workspace.autostart_schedule)}</Typography>
        <Typography>{Language.autoStartDisplay(workspace.autostart_schedule)}</Typography>
      </Box>

      <Box mt={2}>
        <Typography variant="h6">{Language.autoStopLabel}</Typography>
        <Typography data-chromatic="ignore">{Language.autoStopDisplay(workspace)}</Typography>
      </Box>
    </WorkspaceSection>
  )
}
