import { CircularProgress, makeStyles } from "@material-ui/core"
import React from "react"

export const useStyles = makeStyles(() => ({
  root: {
    position: "absolute",
    top: "0",
    left: "0",
    right: "0",
    bottom: "0",
  },
}))

export const FullScreenLoader: React.FC = () => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <CircularProgress />
    </div>
  )
}
