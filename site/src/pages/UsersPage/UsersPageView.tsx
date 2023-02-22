import { PaginationWidget } from "components/PaginationWidget/PaginationWidget"
import { FC } from "react"
import { PaginationMachineRef } from "xServices/pagination/paginationXService"
import * as TypesGen from "../../api/typesGenerated"
import { SearchBarWithFilter } from "../../components/SearchBarWithFilter/SearchBarWithFilter"
import { UsersTable } from "../../components/UsersTable/UsersTable"
import { userFilterQuery } from "../../util/filters"

export const Language = {
  activeUsersFilterName: "Active users",
  allUsersFilterName: "All users",
}
export interface UsersPageViewProps {
  users?: TypesGen.User[]
  count?: number
  roles?: TypesGen.AssignableRoles[]
  filter?: string
  error?: unknown
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  isLoading?: boolean
  onSuspendUser: (user: TypesGen.User) => void
  onDeleteUser: (user: TypesGen.User) => void
  onListWorkspaces: (user: TypesGen.User) => void
  onActivateUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (
    user: TypesGen.User,
    roles: TypesGen.Role["name"][],
  ) => void
  onFilter: (query: string) => void
  paginationRef: PaginationMachineRef
  isNonInitialPage: boolean
  actorID: string
}

export const UsersPageView: FC<React.PropsWithChildren<UsersPageViewProps>> = ({
  users,
  count,
  roles,
  onSuspendUser,
  onDeleteUser,
  onListWorkspaces,
  onActivateUser,
  onResetUserPassword,
  onUpdateUserRoles,
  error,
  isUpdatingUserRoles,
  canEditUsers,
  isLoading,
  filter,
  onFilter,
  paginationRef,
  isNonInitialPage,
  actorID,
}) => {
  const presetFilters = [
    { query: userFilterQuery.active, name: Language.activeUsersFilterName },
    { query: userFilterQuery.all, name: Language.allUsersFilterName },
  ]

  return (
    <>
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
        onDeleteUser={onDeleteUser}
        onListWorkspaces={onListWorkspaces}
        onActivateUser={onActivateUser}
        onResetUserPassword={onResetUserPassword}
        onUpdateUserRoles={onUpdateUserRoles}
        isUpdatingUserRoles={isUpdatingUserRoles}
        canEditUsers={canEditUsers}
        isLoading={isLoading}
        isNonInitialPage={isNonInitialPage}
        actorID={actorID}
      />

      <PaginationWidget numRecords={count} paginationRef={paginationRef} />
    </>
  )
}
