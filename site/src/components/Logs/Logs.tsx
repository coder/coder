import { makeStyles, Theme } from "@material-ui/core/styles"
import { LogLevel } from "api/typesGenerated"
import dayjs from "dayjs"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"

export interface Line {
  time: string
  output: string
  level: LogLevel
}

export interface LogsProps {
  lines: Line[]
  hideTimestamps?: boolean
  lineNumbers?: boolean
  className?: string
}

export const Logs: FC<React.PropsWithChildren<LogsProps>> = ({
  hideTimestamps,
  lines,
  lineNumbers,
  className = "",
}) => {
  const styles = useStyles({
    lineNumbers: Boolean(lineNumbers),
  })

  return (
    <div className={combineClasses([className, styles.root])}>
      <div className={styles.scrollWrapper}>
        {lines.map((line, idx) => (
          <div className={combineClasses([styles.line, line.level])} key={idx}>
            {!hideTimestamps && (
              <>
                <span className={styles.time}>
                  {lineNumbers
                    ? idx + 1
                    : dayjs(line.time).format(`HH:mm:ss.SSS`)}
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

export const logLineHeight = 20

export const LogLine: FC<{
  line: Line
  hideTimestamp?: boolean
  number?: number
  style?: React.CSSProperties
}> = ({ line, hideTimestamp, number, style }) => {
  const styles = useStyles({
    lineNumbers: Boolean(number),
  })

  return (
    <div className={combineClasses([styles.line, line.level])} style={style}>
      {!hideTimestamp && (
        <>
          <span className={styles.time}>
            {number ? number : dayjs(line.time).format(`HH:mm:ss.SSS`)}
          </span>
          <span className={styles.space}>&nbsp;&nbsp;&nbsp;&nbsp;</span>
        </>
      )}
      <span>{line.output}</span>
    </div>
  )
}

const useStyles = makeStyles<
  Theme,
  {
    lineNumbers: boolean
  }
>((theme) => ({
  root: {
    minHeight: 156,
    fontSize: 13,
    padding: theme.spacing(2, 0),
    borderRadius: theme.shape.borderRadius,
    overflowX: "auto",
    background: theme.palette.background.default,
  },
  scrollWrapper: {
    width: "fit-content",
  },
  line: {
    wordBreak: "break-all",
    color: theme.palette.text.primary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    height: ({ lineNumbers }) => (lineNumbers ? logLineHeight : "auto"),
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
    width: ({ lineNumbers }) => theme.spacing(lineNumbers ? 3.5 : 12.5),
    whiteSpace: "pre",
    display: "inline-block",
    color: theme.palette.text.secondary,
  },
}))
