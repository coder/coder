import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { isApiError, mapApiErrorToFieldErrors } from "../../../api/errors"
import { Section } from "../../../components/Section/Section"
import { SecurityForm } from "../../../components/SettingsSecurityForm/SettingsSecurityForm"
import { XServiceContext } from "../../../xServices/StateContext"

export const Language = {
  title: "Security",
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
      <SecurityForm
        error={hasUnknownError ? Language.unknownError : undefined}
        formErrors={formErrors}
        isLoading={authState.matches("signedIn.profile.updatingProfile")}
        initialValues={{ old_password: "", password: "", confirm_password: "" }}
        onSubmit={(data) => {
          authSend({
            type: "UPDATE_PASSWORD",
            data,
          })
        }}
      />
    </Section>
  )
}
