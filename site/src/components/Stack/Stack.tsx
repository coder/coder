import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { combineClasses } from "../../util/combineClasses"

type Direction = "column" | "row"

interface StyleProps {
  direction: Direction
  spacing: number
}

const useStyles = makeStyles((theme) => ({
  stack: {
    display: "flex",
    flexDirection: ({ direction }: StyleProps) => direction,
    gap: ({ spacing }: StyleProps) => theme.spacing(spacing),
  },
}))

export interface StackProps {
  className?: string
  direction?: Direction
  spacing?: number
}

export const Stack: React.FC<StackProps> = ({ children, className, direction = "column", spacing = 2 }) => {
  const styles = useStyles({ spacing, direction })

  return <div className={combineClasses([styles.stack, className])}>{children}</div>
}
