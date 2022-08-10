import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { useNavigate } from "react-router-dom"
import { pageTitle } from "util/page"
import { setupMachine } from "xServices/setup/setupXService"
import { SetupPageView } from "./SetupPageView"

export const SetupPage: FC = () => {
  const navigate = useNavigate()
  const [setupState, setupSend] = useMachine(setupMachine, {
    actions: {
      redirectToWorkspacesPage: () => {
        navigate("/workspaces")
      },
    },
  })
  const { createFirstUserFormErrors, createFirstUserErrorMessage } = setupState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Setup your account")}</title>
      </Helmet>
      <SetupPageView
        formErrors={createFirstUserFormErrors}
        genericError={createFirstUserErrorMessage}
        onSubmit={(firstUser) => {
          setupSend({ type: "CREATE_FIRST_USER", firstUser })
        }}
      />
    </>
  )
}
