import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../components/Header/Header"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  newUserButton: "New user",
}

export interface UsersPageViewProps {
  users?: TypesGen.User[]
  roles?: TypesGen.Role[]
  error?: unknown
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  canCreateUser?: boolean
  isLoading?: boolean
  openUserCreationDialog: () => void
  onSuspendUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (user: TypesGen.User, roles: TypesGen.Role["name"][]) => void
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
  canEditUsers,
  canCreateUser,
  isLoading,
}) => {
  const newUserAction = canCreateUser ? { text: Language.newUserButton, onClick: openUserCreationDialog } : undefined
  return (
    <Stack spacing={4}>
      <Header title={Language.pageTitle} action={newUserAction} />
      <Margins>
        {error ? (
          <ErrorSummary error={error} />
        ) : (
          <UsersTable
            users={users}
            roles={roles}
            onSuspendUser={onSuspendUser}
            onResetUserPassword={onResetUserPassword}
            onUpdateUserRoles={onUpdateUserRoles}
            isUpdatingUserRoles={isUpdatingUserRoles}
            canEditUsers={canEditUsers}
            isLoading={isLoading}
          />
        )}
      </Margins>
    </Stack>
  )
}
