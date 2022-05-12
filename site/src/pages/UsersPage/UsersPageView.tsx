import React from "react"
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
  users: TypesGen.User[]
  openUserCreationDialog: () => void
  onSuspendUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (user: TypesGen.User, roles: TypesGen.Role["name"][]) => void
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
