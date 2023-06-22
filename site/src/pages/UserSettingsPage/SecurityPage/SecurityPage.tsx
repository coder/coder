import { useMachine } from "@xstate/react"
import { useMe } from "hooks/useMe"
import { FC } from "react"
import { userSecuritySettingsMachine } from "xServices/userSecuritySettings/userSecuritySettingsXService"
import { Section } from "../../../components/SettingsLayout/Section"
import { SecurityForm } from "../../../components/SettingsSecurityForm/SettingsSecurityForm"
import { useQuery } from "@tanstack/react-query"
import { getAuthMethods } from "api/api"
import {
  SingleSignOnSection,
  useSingleSignOnSection,
} from "./SingleSignOnSection"
import { Loader } from "components/Loader/Loader"
import { Stack } from "components/Stack/Stack"

export const SecurityPage: FC = () => {
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
  const { data: authMethods } = useQuery({
    queryKey: ["authMethods"],
    queryFn: getAuthMethods,
  })
  const singleSignOnSection = useSingleSignOnSection()

  if (!authMethods) {
    return <Loader />
  }

  return (
    <Stack spacing={6}>
      <Section title="Security" description="Update your account password">
        <SecurityForm
          disabled={authMethods.me_login_type !== "password"}
          updateSecurityError={error}
          isLoading={securityState.matches("updatingSecurity")}
          initialValues={{
            old_password: "",
            password: "",
            confirm_password: "",
          }}
          onSubmit={(data) => {
            securitySend({
              type: "UPDATE_SECURITY",
              data,
            })
          }}
        />
      </Section>
      <SingleSignOnSection authMethods={authMethods} {...singleSignOnSection} />
    </Stack>
  )
}

export default SecurityPage
