import { useMachine } from "@xstate/react"
import { useMe } from "hooks/useMe"
import React from "react"
import { userSecuritySettingsMachine } from "xServices/userSecuritySettings/userSecuritySettingsXService"
import { Section } from "../../../components/Section/Section"
import { SecurityForm } from "../../../components/SettingsSecurityForm/SettingsSecurityForm"

export const Language = {
  title: "Security",
}

export const SecurityPage: React.FC = () => {
  const me = useMe()
  const [securityState, securitySend] = useMachine(
    userSecuritySettingsMachine,
    {
      context: {
        userId: me.id,
      },
    },
  )
  const { error } = securityState.context

  return (
    <Section title={Language.title}>
      <SecurityForm
        updateSecurityError={error}
        isLoading={securityState.matches("updatingSecurity")}
        initialValues={{ old_password: "", password: "", confirm_password: "" }}
        onSubmit={(data) => {
          securitySend({
            type: "UPDATE_SECURITY",
            data,
          })
        }}
      />
    </Section>
  )
}

export default SecurityPage
