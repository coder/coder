import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { isApiError, mapApiErrorToFieldErrors } from "../../../api/errors"
import { AccountForm } from "../../../components/PreferencesAccountForm/PreferencesAccountForm"
import { Section } from "../../../components/Section/Section"
import { XServiceContext } from "../../../xServices/StateContext"

export const Language = {
  title: "Account",
  description: "Update your display name, email, and username.",
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
    <Section title={Language.title} description={Language.description}>
      <AccountForm
        error={hasUnknownError ? Language.unknownError : undefined}
        formErrors={formErrors}
        isLoading={authState.matches("signedIn.profile.updatingProfile")}
        initialValues={{ username: me.username, email: me.email }}
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
