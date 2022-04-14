import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { getFormErrorsFromApiError } from "../../api"
import { AccountForm } from "../../components/Preferences/AccountForm"
import { Section } from "../../components/Section"
import { XServiceContext } from "../../xServices/StateContext"

const Language = {
  title: "Account",
  description: "Update your display name, email, and username.",
}

export const PreferencesAccountPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me } = authState.context
  const formErrors = getFormErrorsFromApiError(authState.context.updateProfileError)

  if (!me) {
    throw new Error("No current user found")
  }

  return (
    <>
      <Section title={Language.title} description={Language.description}>
        <AccountForm
          errors={formErrors}
          isLoading={authState.matches("signedIn.profile.updatingProfile")}
          initialValues={{ name: me.name ?? "", username: me.username, email: me.email }}
          onSubmit={async (data) => {
            authSend({
              type: "UPDATE_PROFILE",
              data,
            })
          }}
        />
      </Section>
    </>
  )
}
