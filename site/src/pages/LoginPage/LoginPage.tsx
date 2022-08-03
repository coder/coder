import { makeStyles } from "@material-ui/core/styles"
import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { Navigate, useLocation } from "react-router-dom"
import { Footer } from "../../components/Footer/Footer"
import { LoginErrors, SignInForm } from "../../components/SignInForm/SignInForm"
import { pageTitle } from "../../util/page"
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

interface LocationState {
  isRedirect: boolean
}

export const LoginPage: React.FC = () => {
  const styles = useStyles()
  const location = useLocation()
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const isLoading = authState.hasTag("loading")
  const redirectTo = retrieveRedirect(location.search)
  const locationState = location.state ? (location.state as LocationState) : null
  const isRedirected = locationState ? locationState.isRedirect : false

  const onSubmit = async ({ email, password }: { email: string; password: string }) => {
    authSend({ type: "SIGN_IN", email, password })
  }

  const { authError, getUserError, checkPermissionsError, getMethodsError } = authState.context

  if (authState.matches("signedIn")) {
    return <Navigate to={redirectTo} replace />
  } else {
    return (
      <div className={styles.root}>
        <Helmet>
          <title>{pageTitle("Login")}</title>
        </Helmet>
        <div className={styles.layout}>
          <div className={styles.container}>
            <SignInForm
              authMethods={authState.context.methods}
              redirectTo={redirectTo}
              isLoading={isLoading}
              loginErrors={{
                [LoginErrors.AUTH_ERROR]: authError,
                [LoginErrors.GET_USER_ERROR]: isRedirected ? getUserError : null,
                [LoginErrors.CHECK_PERMISSIONS_ERROR]: checkPermissionsError,
                [LoginErrors.GET_METHODS_ERROR]: getMethodsError,
              }}
              onSubmit={onSubmit}
            />
          </div>

          <Footer />
        </div>
      </div>
    )
  }
}
