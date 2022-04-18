import Box from "@material-ui/core/Box"
import Typography from "@material-ui/core/Typography"
import cronstrue from "cronstrue"
import React from "react"
import { extractTimezone, stripTimezone } from "../../util/schedule"
import { WorkspaceSection } from "./WorkspaceSection"

const Language = {
  autoStartLabel: (schedule: string): string => {
    const prefix = "Workspace start"

    if (schedule) {
      return `${prefix} (${extractTimezone(schedule)})`
    } else {
      return prefix
    }
  },
  autoStopLabel: (schedule: string): string => {
    const prefix = "Workspace shutdown"

    if (schedule) {
      return `${prefix} (${extractTimezone(schedule)})`
    } else {
      return prefix
    }
  },
  cronHumanDisplay: (schedule: string): string => {
    if (schedule) {
      return cronstrue.toString(stripTimezone(schedule), { throwExceptionOnParseError: false })
    }
    return "Manual"
  },
}

export interface WorkspaceScheduleProps {
  autostart: string
  autostop: string
}

/**
 * WorkspaceSchedule displays a workspace schedule in a human-readable format
 *
 * @remarks Visual Component
 */
export const WorkspaceSchedule: React.FC<WorkspaceScheduleProps> = ({ autostart, autostop }) => {
  return (
    <WorkspaceSection title="Workspace schedule">
      <Box mt={2}>
        <Typography variant="h6">{Language.autoStartLabel(autostart)}</Typography>
        <Typography>{Language.cronHumanDisplay(autostart)}</Typography>
      </Box>

      <Box mt={2}>
        <Typography variant="h6">{Language.autoStopLabel(autostop)}</Typography>
        <Typography>{Language.cronHumanDisplay(autostop)}</Typography>
      </Box>
    </WorkspaceSection>
  )
}
