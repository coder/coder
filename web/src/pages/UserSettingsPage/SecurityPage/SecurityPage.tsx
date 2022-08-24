import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { Section } from "../../../components/Section/Section"
import { SecurityForm } from "../../../components/SettingsSecurityForm/SettingsSecurityForm"
import { XServiceContext } from "../../../xServices/StateContext"

export const Language = {
  title: "Security",
}

export const SecurityPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me, updateSecurityError } = authState.context

  if (!me) {
    throw new Error("No current user found")
  }

  return (
    <Section title={Language.title}>
      <SecurityForm
        updateSecurityError={updateSecurityError}
        isLoading={authState.matches("signedIn.security.updatingSecurity")}
        initialValues={{ old_password: "", password: "", confirm_password: "" }}
        onSubmit={(data) => {
          authSend({
            type: "UPDATE_SECURITY",
            data,
          })
        }}
      />
    </Section>
  )
}
