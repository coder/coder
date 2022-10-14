import { useActor } from "@xstate/react"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { SignInLayout } from "components/SignInLayout/SignInLayout"
import React, { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { Navigate, useLocation } from "react-router-dom"
import { LoginErrors, SignInForm } from "../../components/SignInForm/SignInForm"
import { pageTitle } from "../../util/page"
import { retrieveRedirect } from "../../util/redirect"
import { XServiceContext } from "../../xServices/StateContext"

interface LocationState {
  isRedirect: boolean
}

export const LoginPage: React.FC = () => {
  const location = useLocation()
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const isLoading = authState.hasTag("loading")
  const redirectTo = retrieveRedirect(location.search)
  const locationState = location.state
    ? (location.state as LocationState)
    : null
  const isRedirected = locationState ? locationState.isRedirect : false
  const { authError, getUserError, checkPermissionsError, getMethodsError } =
    authState.context

  const onSubmit = async ({
    email,
    password,
  }: {
    email: string
    password: string
  }) => {
    authSend({ type: "SIGN_IN", email, password })
  }

  if (authState.matches("signedIn")) {
    return <Navigate to={redirectTo} replace />
  } else if (authState.matches("waitingForTheFirstUser")) {
    return <Navigate to="/setup" />
  } else {
    return (
      <>
        <Helmet>
          <title>{pageTitle("Login")}</title>
        </Helmet>
        {authState.hasTag("loading") ? (
          <FullScreenLoader />
        ) : (
          <SignInLayout>
            <SignInForm
              authMethods={authState.context.methods}
              redirectTo={redirectTo}
              isLoading={isLoading}
              loginErrors={{
                [LoginErrors.AUTH_ERROR]: authError,
                [LoginErrors.GET_USER_ERROR]: isRedirected
                  ? getUserError
                  : null,
                [LoginErrors.CHECK_PERMISSIONS_ERROR]: checkPermissionsError,
                [LoginErrors.GET_METHODS_ERROR]: getMethodsError,
              }}
              onSubmit={onSubmit}
            />
          </SignInLayout>
        )}
      </>
    )
  }
}

export default LoginPage
