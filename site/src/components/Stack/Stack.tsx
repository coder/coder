import { makeStyles } from "@material-ui/core/styles"
import { CSSProperties } from "@material-ui/core/styles/withStyles"
import { FC, PropsWithChildren } from "react"
import { combineClasses } from "../../util/combineClasses"

type Direction = "column" | "row"

export interface StackProps {
  className?: string
  direction?: Direction
  spacing?: number
  alignItems?: CSSProperties["alignItems"]
  justifyContent?: CSSProperties["justifyContent"]
}

type StyleProps = Omit<StackProps, "className">

const useStyles = makeStyles((theme) => ({
  stack: {
    display: "flex",
    flexDirection: ({ direction }: StyleProps) => direction,
    gap: ({ spacing }: StyleProps) => spacing && theme.spacing(spacing),
    alignItems: ({ alignItems }: StyleProps) => alignItems,
    justifyContent: ({ justifyContent }: StyleProps) => justifyContent,

    [theme.breakpoints.down("sm")]: {
      width: "100%",
    },
  },
}))

export const Stack: FC<PropsWithChildren<StackProps>> = ({
  children,
  className,
  direction = "column",
  spacing = 2,
  alignItems,
  justifyContent,
}) => {
  const styles = useStyles({ spacing, direction, alignItems, justifyContent })

  return <div className={combineClasses([styles.stack, className])}>{children}</div>
}
