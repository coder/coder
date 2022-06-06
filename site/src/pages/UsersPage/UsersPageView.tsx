import Button from "@material-ui/core/Button"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Margins } from "../../components/Margins/Margins"
import { PageHeader, PageHeaderActions, PageHeaderTitle } from "../../components/PageHeader/PageHeader"
import { UsersTable } from "../../components/UsersTable/UsersTable"

export const Language = {
  pageTitle: "Users",
  createButton: "New user",
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

export const UsersPageView: FC<UsersPageViewProps> = ({
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
  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>Users</PageHeaderTitle>
        <PageHeaderActions>
          {canCreateUser && (
            <Button onClick={openUserCreationDialog} startIcon={<AddCircleOutline />}>
              {Language.createButton}
            </Button>
          )}
        </PageHeaderActions>
      </PageHeader>

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
  )
}
