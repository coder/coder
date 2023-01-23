import { useMachine } from "@xstate/react"
import { useAuth } from "components/AuthProvider/AuthProvider"
import { FC, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { setupMachine } from "xServices/setup/setupXService"
import { SetupPageView } from "./SetupPageView"

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
  const { createFirstUserFormErrors, createFirstUserErrorMessage } =
    setupState.context

  useEffect(() => {
    if (authState.matches("signedIn")) {
      window.location.assign("/workspaces")
    }
  }, [authState])

  return (
    <>
      <Helmet>
        <title>{pageTitle("Set up your account")}</title>
      </Helmet>
      <SetupPageView
        isLoading={setupState.hasTag("loading")}
        formErrors={createFirstUserFormErrors}
        genericError={createFirstUserErrorMessage}
        onSubmit={(firstUser) => {
          setupSend({ type: "CREATE_FIRST_USER", firstUser })
        }}
      />
    </>
  )
}
