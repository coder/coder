import { makeStyles } from "@material-ui/core/styles"
import { CSSProperties } from "@material-ui/core/styles/withStyles"
import { FC } from "react"
import { combineClasses } from "../../util/combineClasses"

type Direction = "column" | "row"

interface StyleProps {
  direction: Direction
  spacing: number
  alignItems?: CSSProperties["alignItems"]
}

const useStyles = makeStyles((theme) => ({
  stack: {
    display: "flex",
    flexDirection: ({ direction }: StyleProps) => direction,
    gap: ({ spacing }: StyleProps) => theme.spacing(spacing),
    alignItems: ({ alignItems }: StyleProps) => alignItems,

    [theme.breakpoints.down("sm")]: {
      width: "100%",
    },
  },
}))

export interface StackProps {
  className?: string
  direction?: Direction
  spacing?: number
  alignItems?: CSSProperties["alignItems"]
  children: React.ReactNode
}

export const Stack: FC<StackProps> = ({
  children,
  className,
  direction = "column",
  spacing = 2,
  alignItems,
}) => {
  const styles = useStyles({ spacing, direction, alignItems })

  return <div className={combineClasses([styles.stack, className])}>{children}</div>
}
