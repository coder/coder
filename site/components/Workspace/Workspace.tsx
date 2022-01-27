import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"

import * as API from "../../api"

export interface WorkspaceProps {
  workspace: API.Workspace
}

export const Workspace: React.FC<WorkspaceProps> = ({ workspace }) => {
  const styles = useStyles()

  return <div className={styles.root}>
    <Paper elevation={0} className={styles.section}>
      <div>Hello</div>
    </Paper>
    <div className={styles.horizontal}>
      <Paper elevation={0} className={styles.sideBar}>
        <div>Apps</div>
      </Paper>
      <Paper elevation={0} className={styles.main}>
        <div>Build stuff</div>
      </Paper>
    </div>
  </div>
}

namespace Constants {
  export const CardRadius = 8
  export const CardPadding = 20
}

export const useStyles = makeStyles((theme) => {

  const common = {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: Constants.CardRadius,
    margin: theme.spacing(1),
    padding: Constants.CardPadding
  }

  return {
    root: {
      display: "flex",
      flexDirection: "column"
    },
    horizontal: {
      display: "flex",
      flexDirection: "row"
    },
    section: common,
    sideBar: {
      ...common,
      width: "400px"
    },
    main: {
      ...common,
      flex: 1
    }
  }
})