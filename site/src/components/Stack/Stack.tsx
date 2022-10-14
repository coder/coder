import { makeStyles } from "@material-ui/core/styles"
import { CSSProperties } from "@material-ui/core/styles/withStyles"
import { FC } from "react"
import { ReactNode } from "react-markdown/lib/react-markdown"
import { combineClasses } from "../../util/combineClasses"

type Direction = "column" | "row"

export type StackProps = {
  className?: string
  direction?: Direction
  spacing?: number
  alignItems?: CSSProperties["alignItems"]
  justifyContent?: CSSProperties["justifyContent"]
} & React.HTMLProps<HTMLDivElement>

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

export const Stack: FC<StackProps & { children: ReactNode | ReactNode[] }> = ({
  children,
  className,
  direction = "column",
  spacing = 2,
  alignItems,
  justifyContent,
  ...divProps
}) => {
  const styles = useStyles({
    spacing,
    direction,
    alignItems,
    justifyContent,
  })

  return (
    <div {...divProps} className={combineClasses([styles.stack, className])}>
      {children}
    </div>
  )
}
