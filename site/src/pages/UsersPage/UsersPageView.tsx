import Button from "@material-ui/core/Button"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { PageHeader, PageHeaderTitle } from "../../components/PageHeader/PageHeader"
import { SearchBarWithFilter } from "../../components/SearchBarWithFilter/SearchBarWithFilter"
import { UsersTable } from "../../components/UsersTable/UsersTable"
import { userFilterQuery } from "../../util/filters"

export const Language = {
  pageTitle: "Users",
  createButton: "New user",
  activeUsersFilterName: "Active users",
  allUsersFilterName: "All users",
}

export interface UsersPageViewProps {
  users?: TypesGen.User[]
  roles?: TypesGen.Role[]
  filter?: string
  error?: unknown
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  canCreateUser?: boolean
  isLoading?: boolean
  openUserCreationDialog: () => void
  onSuspendUser: (user: TypesGen.User) => void
  onActivateUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (user: TypesGen.User, roles: TypesGen.Role["name"][]) => void
  onFilter: (query: string) => void
}

export const UsersPageView: FC<React.PropsWithChildren<UsersPageViewProps>> = ({
  users,
  roles,
  openUserCreationDialog,
  onSuspendUser,
  onActivateUser,
  onResetUserPassword,
  onUpdateUserRoles,
  error,
  isUpdatingUserRoles,
  canEditUsers,
  canCreateUser,
  isLoading,
  filter,
  onFilter,
}) => {
  const presetFilters = [
    { query: userFilterQuery.active, name: Language.activeUsersFilterName },
    { query: userFilterQuery.all, name: Language.allUsersFilterName },
  ]

  return (
    <Margins>
      <PageHeader
        actions={
          canCreateUser ? (
            <Button onClick={openUserCreationDialog} startIcon={<AddCircleOutline />}>
              {Language.createButton}
            </Button>
          ) : undefined
        }
      >
        <PageHeaderTitle>{Language.pageTitle}</PageHeaderTitle>
      </PageHeader>

      <SearchBarWithFilter
        filter={filter}
        onFilter={onFilter}
        presetFilters={presetFilters}
        error={error}
      />

      <UsersTable
        users={users}
        roles={roles}
        onSuspendUser={onSuspendUser}
        onActivateUser={onActivateUser}
        onResetUserPassword={onResetUserPassword}
        onUpdateUserRoles={onUpdateUserRoles}
        isUpdatingUserRoles={isUpdatingUserRoles}
        canEditUsers={canEditUsers}
        isLoading={isLoading}
      />
    </Margins>
  )
}
