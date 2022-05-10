import React from "react"
import { UserResponse } from "../../api/types"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../components/Header/Header"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  newUserButton: "New User",
}

export interface UsersPageViewProps {
  users: UserResponse[]
  openUserCreationDialog: () => void
  onSuspendUser: (user: UserResponse) => void
  onResetUserPassword: (user: UserResponse) => void
  error?: unknown
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({
  users,
  openUserCreationDialog,
  onSuspendUser,
  onResetUserPassword,
  error,
}) => {
  return (
    <Stack spacing={4}>
      <Header title={Language.pageTitle} action={{ text: Language.newUserButton, onClick: openUserCreationDialog }} />
      <Margins>
        {error ? (
          <ErrorSummary error={error} />
        ) : (
          <UsersTable users={users} onSuspendUser={onSuspendUser} onResetUserPassword={onResetUserPassword} />
        )}
      </Margins>
    </Stack>
  )
}
