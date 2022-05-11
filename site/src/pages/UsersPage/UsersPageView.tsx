import React from "react"
import { UserResponse } from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"
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
  onUpdateUserRoles: (user: UserResponse, roles: TypesGen.Role["name"][]) => void
  roles: TypesGen.Role[]
  error?: unknown
  isUpdatingUserRoles?: boolean
}

export const UsersPageView: React.FC<UsersPageViewProps> = ({
  users,
  roles,
  openUserCreationDialog,
  onSuspendUser,
  onResetUserPassword,
  onUpdateUserRoles,
  error,
  isUpdatingUserRoles,
}) => {
  return (
    <Stack spacing={4}>
      <Header title={Language.pageTitle} action={{ text: Language.newUserButton, onClick: openUserCreationDialog }} />
      <Margins>
        {error ? (
          <ErrorSummary error={error} />
        ) : (
          <UsersTable
            users={users}
            onSuspendUser={onSuspendUser}
            onResetUserPassword={onResetUserPassword}
            onUpdateUserRoles={onUpdateUserRoles}
            roles={roles}
            isUpdatingUserRoles={isUpdatingUserRoles}
          />
        )}
      </Margins>
    </Stack>
  )
}
