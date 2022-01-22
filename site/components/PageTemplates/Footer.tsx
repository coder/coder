import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"

export const Footer: React.FC = ({ children }) => {
  const styles = useFooterStyles()

  return (
    <div className={styles.root}>
      {children}
      <div className={styles.copyRight}>
        <Typography color="textSecondary" variant="caption">
          {`Copyright \u00a9 ${new Date().getFullYear()} Coder Technologies, Inc. All rights reserved.`}
        </Typography>
      </div>
      <div className={styles.version}>
        <Typography color="textSecondary" variant="caption">
          v2 0.0.0-prototype
        </Typography>
      </div>
    </div>
  )
}

const useFooterStyles = makeStyles((theme) => ({
  root: {
    textAlign: "center",
    marginBottom: theme.spacing(5),
    flex: "0",
  },
  copyRight: {
    backgroundColor: theme.palette.background.default,
    margin: theme.spacing(0.25),
  },
  version: {
    backgroundColor: theme.palette.background.default,
    margin: theme.spacing(0.25),
  },
}))
