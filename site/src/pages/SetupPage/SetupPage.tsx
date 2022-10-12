import { useActor, useMachine } from "@xstate/react"
import { FC, useContext, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate } from "react-router-dom"
import { pageTitle } from "util/page"
import { setupMachine } from "xServices/setup/setupXService"
import { XServiceContext } from "xServices/StateContext"
import { SetupPageView } from "./SetupPageView"

export const SetupPage: FC = () => {
  const navigate = useNavigate()
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
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
      return navigate("/workspaces")
    }
  }, [authState, navigate])

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
