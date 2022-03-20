import { makeStyles } from "@material-ui/core/styles"
import React, { useContext, useEffect, useState } from "react"
import { getApiKey } from "../api"
import { CliAuthToken } from "../components/SignIn"

import { FullScreenLoader } from "../components/Loader/FullScreenLoader"
import { useActor } from "@xstate/react"
import { XServiceContext } from "../xServices/StateContext"

export const CliAuthenticationPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [userState] = useActor(xServices.userXService)
  const { me } = userState.context

  const styles = useStyles()

  const [apiKey, setApiKey] = useState<string | null>(null)

  useEffect(() => {
    if (me?.id) {
      void getApiKey().then(({ key }) => {
        setApiKey(key)
      })
    }
  }, [me?.id])

  if (!apiKey) {
    return <FullScreenLoader />
  }

  return (
    <div className={styles.root}>
      <CliAuthToken sessionToken={apiKey} />
    </div>
  )
}

const useStyles = makeStyles(() => ({
  root: {
    width: "100vw",
    height: "100vh",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
}))
