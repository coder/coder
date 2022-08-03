import { makeStyles } from "@material-ui/core/styles"
import dayjs from "dayjs"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"

interface Line {
  time: string
  output: string
}

export interface LogsProps {
  lines: Line[]
  className?: string
}

export const Logs: FC<React.PropsWithChildren<LogsProps>> = ({ lines, className = "" }) => {
  const styles = useStyles()

  return (
    <div className={combineClasses([className, styles.root])}>
      {lines.map((line, idx) => (
        <div className={styles.line} key={idx}>
          <div className={styles.time}>{dayjs(line.time).format(`HH:mm:ss.SSS`)}</div>
          <div>{line.output}</div>
        </div>
      ))}
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
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    overflowX: "auto",
  },
  line: {
    display: "flex",
    alignItems: "baseline",
  },
  time: {
    width: theme.spacing(12.5),
    marginRight: theme.spacing(3),
    flexShrink: 0,
  },
}))
