import { makeStyles } from "@material-ui/core/styles"
import React, { useEffect, useState } from "react"
import { getApiKey } from "../api"
import { CliAuthToken } from "../components/SignIn"

import { FullScreenLoader } from "../components/Loader/FullScreenLoader"
import { useUser } from "../contexts/UserContext"
import { Typography } from "@material-ui/core"

const CliAuthenticationPage: React.FC = () => {
  const { me } = useUser(true)
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.headingContainer}>
        <Typography variant="h4">404</Typography>
      </div>
      <Typography variant="body2">This page could not be found.</Typography>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    width: "100vw",
    height: "100vh",
    display: "flex",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
  },
  headingContainer: {
    margin: theme.spacing(1),
    padding: theme.spacing(1),
    borderRight: theme.palette.divider,
  },
}))

export default CliAuthenticationPage
