import { makeStyles } from "@material-ui/core/styles"
import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { SignInForm } from "./../components/SignIn"
import { Navigate, useLocation } from "react-router-dom"
import { XServiceContext } from "../xServices/StateContext"
import { retrieveRedirect } from "../util/redirect"

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
  const [userState, userSend] = useActor(xServices.userXService)
  const isLoading = userState.hasTag("loading")
  const redirectTo = retrieveRedirect(location.search)
  const authErrorMessage = userState.context.authError ? (userState.context.authError as Error).message : undefined

  const onSubmit = async ({ email, password }: { email: string; password: string }) => {
    userSend({ type: "SIGN_IN", email, password })
  }

  if (userState.matches("signedIn")) {
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
