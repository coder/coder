import { PaginationWidget } from "components/PaginationWidget/PaginationWidget"
import { ComponentProps, FC } from "react"
import { PaginationMachineRef } from "xServices/pagination/paginationXService"
import * as TypesGen from "../../api/typesGenerated"
import { UsersTable } from "../../components/UsersTable/UsersTable"
import { UsersFilter } from "./UsersFilter"
import { PaginationStatus } from "components/PaginationStatus/PaginationStatus"

export const Language = {
  activeUsersFilterName: "Active users",
  allUsersFilterName: "All users",
}
export interface UsersPageViewProps {
  users?: TypesGen.User[]
  count?: number
  roles?: TypesGen.AssignableRoles[]
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  canViewActivity?: boolean
  isLoading?: boolean
  onSuspendUser: (user: TypesGen.User) => void
  onDeleteUser: (user: TypesGen.User) => void
  onListWorkspaces: (user: TypesGen.User) => void
  onViewActivity: (user: TypesGen.User) => void
  onActivateUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (
    user: TypesGen.User,
    roles: TypesGen.Role["name"][],
  ) => void
  filterProps: ComponentProps<typeof UsersFilter>
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
  onViewActivity,
  onActivateUser,
  onResetUserPassword,
  onUpdateUserRoles,
  isUpdatingUserRoles,
  canEditUsers,
  canViewActivity,
  isLoading,
  filterProps,
  paginationRef,
  isNonInitialPage,
  actorID,
}) => {
  return (
    <>
      <UsersFilter {...filterProps} />

      <PaginationStatus
        isLoading={Boolean(isLoading)}
        showing={users?.length ?? 0}
        total={count ?? 0}
        label="users"
      />

      <UsersTable
        users={users}
        roles={roles}
        onSuspendUser={onSuspendUser}
        onDeleteUser={onDeleteUser}
        onListWorkspaces={onListWorkspaces}
        onViewActivity={onViewActivity}
        onActivateUser={onActivateUser}
        onResetUserPassword={onResetUserPassword}
        onUpdateUserRoles={onUpdateUserRoles}
        isUpdatingUserRoles={isUpdatingUserRoles}
        canEditUsers={canEditUsers}
        canViewActivity={canViewActivity}
        isLoading={isLoading}
        isNonInitialPage={isNonInitialPage}
        actorID={actorID}
      />

      <PaginationWidget numRecords={count} paginationRef={paginationRef} />
    </>
  )
}
