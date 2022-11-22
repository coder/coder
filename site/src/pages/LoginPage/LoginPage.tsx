import { useActor } from "@xstate/react"
import { FC, useContext } from "react"
import { Helmet } from "react-helmet-async"
import { Navigate, useLocation } from "react-router-dom"
import { retrieveRedirect } from "../../util/redirect"
import { XServiceContext } from "../../xServices/StateContext"
import { LoginPageView } from "./LoginPageView"

export const LoginPage: FC = () => {
  const location = useLocation()
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const redirectTo = retrieveRedirect(location.search)

  if (authState.matches("signedIn")) {
    return <Navigate to={redirectTo} replace />
  } else if (authState.matches("waitingForTheFirstUser")) {
    return <Navigate to="/setup" />
  } else {
    return (
      <>
        <Helmet>
          <title>Sign in to Coder</title>
        </Helmet>
        <LoginPageView
          context={authState.context}
          isLoading={authState.hasTag("loading")}
          onSignIn={({ email, password }) => {
            authSend({ type: "SIGN_IN", email, password })
          }}
        />
      </>
    )
  }
}

export default LoginPage
