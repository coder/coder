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
  maxWidth?: CSSProperties["maxWidth"]
  wrap?: CSSProperties["flexWrap"]
} & React.HTMLProps<HTMLDivElement>

type StyleProps = Omit<StackProps, "className">

const useStyles = makeStyles((theme) => ({
  stack: {
    display: "flex",
    flexDirection: ({ direction }: StyleProps) => direction,
    gap: ({ spacing }: StyleProps) => spacing && theme.spacing(spacing),
    alignItems: ({ alignItems }: StyleProps) => alignItems,
    justifyContent: ({ justifyContent }: StyleProps) => justifyContent,
    flexWrap: ({ wrap }: StyleProps) => wrap,
    maxWidth: ({ maxWidth }: StyleProps) => maxWidth,

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
  maxWidth,
  wrap,
  ...divProps
}) => {
  const styles = useStyles({
    spacing,
    direction,
    alignItems,
    justifyContent,
    wrap,
    maxWidth,
  })

  return (
    <div {...divProps} className={combineClasses([styles.stack, className])}>
      {children}
    </div>
  )
}
