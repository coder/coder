import { makeStyles } from "@material-ui/core/styles"
import React from "react"

export interface StackProps {
  spacing?: number
}

const useStyles = makeStyles((theme) => ({
  stack: {
    display: "flex",
    flexDirection: "column",
    gap: ({ spacing }: { spacing: number }) => theme.spacing(spacing),
  },
}))

export const Stack: React.FC<StackProps> = ({ children, spacing = 2 }) => {
  const styles = useStyles({ spacing })
  return <div className={styles.stack}>{children}</div>
}
