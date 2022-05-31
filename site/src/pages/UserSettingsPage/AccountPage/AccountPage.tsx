import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { isApiError, mapApiErrorToFieldErrors } from "../../../api/errors"
import { Section } from "../../../components/Section/Section"
import { AccountForm } from "../../../components/SettingsAccountForm/SettingsAccountForm"
import { XServiceContext } from "../../../xServices/StateContext"

export const Language = {
  title: "Account",
  unknownError: "Oops, an unknown error occurred.",
}

export const AccountPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me, updateProfileError } = authState.context
  const hasError = !!updateProfileError
  const formErrors =
    hasError && isApiError(updateProfileError) ? mapApiErrorToFieldErrors(updateProfileError.response.data) : undefined
  const hasUnknownError = hasError && !isApiError(updateProfileError)

  if (!me) {
    throw new Error("No current user found")
  }

  return (
    <Section title={Language.title}>
      <AccountForm
        email={me.email}
        error={hasUnknownError ? Language.unknownError : undefined}
        formErrors={formErrors}
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
