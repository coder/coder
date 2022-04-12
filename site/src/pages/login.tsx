import { makeStyles } from "@material-ui/core/styles"
import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { Navigate, useLocation } from "react-router-dom"
import { retrieveRedirect } from "../util/redirect"
import { XServiceContext } from "../xServices/StateContext"
import { SignInForm } from "./../components/SignIn"

export const useStyles = makeStyles((theme) => ({
  root: {
    height: "100vh",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
  container: {
    marginTop: theme.spacing(-8),
    minWidth: "320px",
    maxWidth: "320px",
  },
}))

export const SignInPage: React.FC = () => {
  const styles = useStyles()
  const location = useLocation()
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const isLoading = authState.hasTag("loading")
  const redirectTo = retrieveRedirect(location.search)
  const authErrorMessage = authState.context.authError ? (authState.context.authError as Error).message : undefined

  const onSubmit = async ({ email, password }: { email: string; password: string }) => {
    authSend({ type: "SIGN_IN", email, password })
  }

  if (authState.matches("signedIn")) {
    return <Navigate to={redirectTo} replace />
  } else {
    return (
      <div className={styles.root}>
        <div className={styles.container}>
          <SignInForm isLoading={isLoading} authErrorMessage={authErrorMessage} onSubmit={onSubmit} />
        </div>
      </div>
    )
  }
}
