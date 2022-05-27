import { makeStyles } from "@material-ui/core/styles"
import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { Navigate, useLocation } from "react-router-dom"
import { Footer } from "../../components/Footer/Footer"
import { SignInForm } from "../../components/SignInForm/SignInForm"
import { retrieveRedirect } from "../../util/redirect"
import { XServiceContext } from "../../xServices/StateContext"

export const useStyles = makeStyles((theme) => ({
  root: {
    height: "100vh",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
  layout: {
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },
  container: {
    marginTop: theme.spacing(-8),
    minWidth: "320px",
    maxWidth: "320px",
  },
}))

export const LoginPage: React.FC = () => {
  const styles = useStyles()
  const location = useLocation()
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const isLoading = authState.hasTag("loading")
  const redirectTo = retrieveRedirect(location.search)
  const authErrorMessage = authState.context.authError ? (authState.context.authError as Error).message : undefined
  const getMethodsError = authState.context.getMethodsError
    ? (authState.context.getMethodsError as Error).message
    : undefined

  const onSubmit = async ({ email, password }: { email: string; password: string }) => {
    authSend({ type: "SIGN_IN", email, password })
  }

  if (authState.matches("signedIn")) {
    return <Navigate to={redirectTo} replace />
  } else {
    return (
      <div className={styles.root}>
        <div className={styles.layout}>
          <div className={styles.container}>
            <SignInForm
              authMethods={authState.context.methods}
              redirectTo={redirectTo}
              isLoading={isLoading}
              authErrorMessage={authErrorMessage}
              methodsErrorMessage={getMethodsError}
              onSubmit={onSubmit}
            />
          </div>

          <Footer />
        </div>
      </div>
    )
  }
}
