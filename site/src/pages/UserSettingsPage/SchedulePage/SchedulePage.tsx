import { FC, useEffect, useState } from "react"
import { Section } from "../../../components/SettingsLayout/Section"
import { AccountForm } from "../../../components/SettingsAccountForm/SettingsAccountForm"
import { useAuth } from "components/AuthProvider/AuthProvider"
import { useMe } from "hooks/useMe"
import { usePermissions } from "hooks/usePermissions"
import { UserQuietHoursScheduleResponse } from "api/typesGenerated"
import * as API from "api/api"

export const SchedulePage: FC = () => {
  const [authState, authSend] = useAuth()
  const me = useMe()
  const permissions = usePermissions()
  const { updateProfileError } = authState.context
  const canEditUsers = permissions && permissions.updateUsers

  const [quietHoursSchedule, setQuietHoursSchedule] = useState<UserQuietHoursScheduleResponse | undefined>(undefined)
  const [quietHoursScheduleError, setQuietHoursScheduleError] = useState<string>("")

  useEffect(() => {
    setQuietHoursSchedule(undefined)
    API.getUserQuietHoursSchedule(me.id)
      .then(response => {
        setQuietHoursSchedule(response)
        setQuietHoursScheduleError("")
      })
      .catch(error => {
        setQuietHoursSchedule(undefined)
        setQuietHoursScheduleError(error.message)
      })
  }, [me.id])

  return (
    <Section title="Schedule" description="Manage your quiet hours schedule">
      <pre>
        {JSON.stringify(quietHoursSchedule, null, 2)}

        {quietHoursScheduleError}
      </pre>
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

export default SchedulePage
