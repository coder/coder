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

export const SecurityPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me, updateSecurityError } = authState.context
  const hasError = !!updateSecurityError
  const formErrors =
    hasError && isApiError(updateSecurityError)
      ? mapApiErrorToFieldErrors(updateSecurityError.response.data)
      : undefined
  const hasUnknownError = hasError && !isApiError(updateSecurityError)

  if (!me) {
    throw new Error("No current user found")
  }

  return (
    <Section title={Language.title}>
      <SecurityForm
        error={hasUnknownError ? Language.unknownError : undefined}
        formErrors={formErrors}
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
