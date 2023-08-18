import { useMachine } from "@xstate/react"
import { useAuth } from "components/AuthProvider/AuthProvider"
import { FC, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { setupMachine } from "xServices/setup/setupXService"
import { SetupPageView } from "./SetupPageView"
import { useNavigate } from "react-router-dom"

export const SetupPage: FC = () => {
  const [authState, authSend] = useAuth()
  const [setupState, setupSend] = useMachine(setupMachine, {
    actions: {
      onCreateFirstUser: ({ firstUser }) => {
        if (!firstUser) {
          throw new Error("First user was not defined.")
        }
        authSend({
          type: "SIGN_IN",
          email: firstUser.email,
          password: firstUser.password,
        })
      },
    },
  })
  const { error } = setupState.context
  const navigate = useNavigate()

  useEffect(() => {
    // If the user is logged in, navigate to the app
    if (authState.matches("signedIn")) {
      navigate("/", { state: { isRedirect: true } })
    }

    // If we've already completed setup, navigate to the login page
    if (
      !authState.matches("loadingInitialAuthData") &&
      !authState.matches("configuringTheFirstUser")
    ) {
      navigate("/login", { state: { isRedirect: true } })
    }
  }, [authState, navigate])

  return (
    <>
      <Helmet>
        <title>{pageTitle("Set up your account")}</title>
      </Helmet>
      <SetupPageView
        isLoading={setupState.hasTag("loading")}
        error={error}
        onSubmit={(firstUser) => {
          setupSend({ type: "CREATE_FIRST_USER", firstUser })
        }}
      />
    </>
  )
}
