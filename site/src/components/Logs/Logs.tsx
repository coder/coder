import { makeStyles } from "@material-ui/core/styles"
import { LogLevel } from "api/typesGenerated"
import dayjs from "dayjs"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"

interface Line {
  time: string
  output: string
  level: LogLevel
}

export interface LogsProps {
  lines: Line[]
  hideTimestamps?: boolean
  className?: string
}

export const Logs: FC<React.PropsWithChildren<LogsProps>> = ({
  hideTimestamps,
  lines,
  className = "",
}) => {
  const styles = useStyles()

  return (
    <div className={combineClasses([className, styles.root])}>
      <div className={styles.scrollWrapper}>
        {lines.map((line, idx) => (
          <div className={combineClasses([styles.line, line.level])} key={idx}>
            {!hideTimestamps && (
              <>
                <span className={styles.time}>
                  {dayjs(line.time).format(`HH:mm:ss.SSS`)}
                </span>
                <span className={styles.space}>&nbsp;&nbsp;&nbsp;&nbsp;</span>
              </>
            )}
            <span>{line.output}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    minHeight: 156,
    background: theme.palette.background.default,
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 13,
    wordBreak: "break-all",
    padding: theme.spacing(2, 0),
    borderRadius: theme.shape.borderRadius,
    overflowX: "auto",
  },
  scrollWrapper: {
    width: "fit-content",
  },
  line: {
    // Whitespace is significant in terminal output for alignment
    whiteSpace: "pre",
    padding: theme.spacing(0, 3),

    "&.error": {
      backgroundColor: theme.palette.error.dark,
    },

    "&.warning": {
      backgroundColor: theme.palette.warning.dark,
    },
  },
  space: {
    userSelect: "none",
  },
  time: {
    userSelect: "none",
    width: theme.spacing(12.5),
    display: "inline-block",
    color: theme.palette.text.secondary,
  },
}))
