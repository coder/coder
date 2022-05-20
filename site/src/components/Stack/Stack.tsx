import { makeStyles } from "@material-ui/core/styles"
import React from "react"

type Direction = "column" | "row"

interface StyleProps {
  spacing: number
  direction: Direction
}

const useStyles = makeStyles((theme) => ({
  stack: {
    display: "flex",
    flexDirection: ({ direction }: StyleProps) => direction,
    gap: ({ spacing }: StyleProps) => theme.spacing(spacing),
  },
}))

export interface StackProps {
  spacing?: number
  direction?: Direction
}

export const Stack: React.FC<StackProps> = ({ children, spacing = 2, direction = "column" }) => {
  const styles = useStyles({ spacing, direction })
  return <div className={styles.stack}>{children}</div>
}
