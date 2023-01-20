import { FC } from "react"
import { Section } from "../../../components/SettingsLayout/Section"
import { AccountForm } from "../../../components/SettingsAccountForm/SettingsAccountForm"
import { useAuth } from "components/AuthProvider/AuthProvider"

export const Language = {
  title: "Account",
}

export const AccountPage: FC = () => {
  const [authState, authSend] = useAuth()
  const { me, permissions, updateProfileError } = authState.context
  const canEditUsers = permissions && permissions.updateUsers

  if (!me) {
    throw new Error("No current user found")
  }

  return (
    <Section title={Language.title} description="Update your account info">
      <AccountForm
        editable={Boolean(canEditUsers)}
        email={me.email}
        updateProfileError={updateProfileError}
        isLoading={authState.matches("signedIn.profile.updatingProfile")}
        initialValues={{
          username: me.username,
        }}
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

export default AccountPage
