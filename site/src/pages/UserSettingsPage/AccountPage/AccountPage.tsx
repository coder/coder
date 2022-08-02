import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { Section } from "../../../components/Section/Section"
import { AccountForm } from "../../../components/SettingsAccountForm/SettingsAccountForm"
import { XServiceContext } from "../../../xServices/StateContext"

export const Language = {
  title: "Account",
}

export const AccountPage: React.FC<React.PropsWithChildren<unknown>> = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me, updateProfileError } = authState.context

  if (!me) {
    throw new Error("No current user found")
  }

  return (
    <Section title={Language.title}>
      <AccountForm
        email={me.email}
        updateProfileError={updateProfileError}
        isLoading={authState.matches("signedIn.profile.updatingProfile")}
        initialValues={{ username: me.username }}
        onSubmit={(data) => {
          authSend({
            type: "UPDATE_PROFILE",
            data,
          })
        }}
      />
    </Section>
  )
}
