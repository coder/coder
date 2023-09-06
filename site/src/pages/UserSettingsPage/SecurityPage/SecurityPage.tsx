import { useMachine } from "@xstate/react"
import { useMe } from "hooks/useMe"
import { ComponentProps, FC } from "react"
import { userSecuritySettingsMachine } from "xServices/userSecuritySettings/userSecuritySettingsXService"
import { Section } from "../../../components/SettingsLayout/Section"
import { SecurityForm } from "./SettingsSecurityForm"
import { useQuery } from "@tanstack/react-query"
import { getAuthMethods, getUserLoginType } from "api/api"
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
  const { data: userLoginType } = useQuery({
    queryKey: ["loginType"],
    queryFn: getUserLoginType,
  })
  const singleSignOnSection = useSingleSignOnSection()

  if (!authMethods || !userLoginType) {
    return <Loader />
  }

  return (
    <SecurityPageView
      security={{
        form: {
          disabled: userLoginType.login_type !== "password",
          error,
          isLoading: securityState.matches("updatingSecurity"),
          onSubmit: (data) => {
            securitySend({
              type: "UPDATE_SECURITY",
              data,
            })
          },
        },
      }}
      oidc={{
        section: {
          authMethods,
          userLoginType,
          ...singleSignOnSection,
        },
      }}
    />
  )
}

export const SecurityPageView = ({
  security,
  oidc,
}: {
  security: {
    form: ComponentProps<typeof SecurityForm>
  }
  oidc?: {
    section: ComponentProps<typeof SingleSignOnSection>
  }
}) => {
  return (
    <Stack spacing={6}>
      <Section title="Security" description="Update your account password">
        <SecurityForm {...security.form} />
      </Section>
      {oidc && <SingleSignOnSection {...oidc.section} />}
    </Stack>
  )
}

export default SecurityPage
