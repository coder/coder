import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { Workspace } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import {
  autoStartDisplay,
  autoStopDisplay,
  extractTimezone,
  Language as ScheduleLanguage,
} from "../../util/schedule"
import { Stack } from "../Stack/Stack"

// REMARK: some plugins depend on utc, so it's listed first. Otherwise they're
//         sorted alphabetically.
dayjs.extend(utc)
dayjs.extend(advancedFormat)
dayjs.extend(duration)
dayjs.extend(relativeTime)
dayjs.extend(timezone)

export const Language = {
  editScheduleLink: "Edit schedule",
  timezoneLabel: "Timezone",
}

export interface WorkspaceScheduleProps {
  workspace: Workspace
  canUpdateWorkspace: boolean
}

export const WorkspaceSchedule: FC<React.PropsWithChildren<WorkspaceScheduleProps>> = ({
  workspace,
  canUpdateWorkspace,
}) => {
  const styles = useStyles()
  const timezone = workspace.autostart_schedule
    ? extractTimezone(workspace.autostart_schedule)
    : dayjs.tz.guess()

  return (
    <div className={styles.schedule}>
      <Stack spacing={2}>
        <div>
          <span className={styles.scheduleLabel}>{Language.timezoneLabel}</span>
          <span className={styles.scheduleValue}>{timezone}</span>
        </div>
        <div>
          <span className={styles.scheduleLabel}>{ScheduleLanguage.autoStartLabel}</span>
          <span className={styles.scheduleValue}>
            {autoStartDisplay(workspace.autostart_schedule)}
          </span>
        </div>
        <div>
          <span className={styles.scheduleLabel}>{ScheduleLanguage.autoStopLabel}</span>
          <Stack direction="row">
            <span className={styles.scheduleValue}>{autoStopDisplay(workspace)}</span>
          </Stack>
        </div>
        {canUpdateWorkspace && (
          <div>
            <Link
              className={styles.scheduleAction}
              component={RouterLink}
              to={`/@${workspace.owner_name}/${workspace.name}/schedule`}
            >
              {Language.editScheduleLink}
            </Link>
          </div>
        )}
      </Stack>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  schedule: {
    fontFamily: MONOSPACE_FONT_FAMILY,
  },
  scheduleLabel: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    color: theme.palette.text.secondary,
  },
  scheduleValue: {
    fontSize: 14,
    marginTop: theme.spacing(0.5),
    display: "inline-block",
    color: theme.palette.text.secondary,
  },
  scheduleAction: {
    cursor: "pointer",
  },
}))
