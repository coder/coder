import { PaginationWidget } from "components/PaginationWidget/PaginationWidget";
import { ComponentProps, FC } from "react";
import { PaginationMachineRef } from "xServices/pagination/paginationXService";
import * as TypesGen from "../../api/typesGenerated";
import { UsersTable } from "./UsersTable/UsersTable";
import { UsersFilter } from "./UsersFilter";
import {
  PaginationStatus,
  TableToolbar,
} from "components/TableToolbar/TableToolbar";

export const Language = {
  activeUsersFilterName: "Active users",
  allUsersFilterName: "All users",
};
export interface UsersPageViewProps {
  users?: TypesGen.User[];
  count?: number;
  roles?: TypesGen.AssignableRoles[];
  isUpdatingUserRoles?: boolean;
  canEditUsers?: boolean;
  oidcRoleSyncEnabled: boolean;
  canViewActivity?: boolean;
  isLoading?: boolean;
  authMethods?: TypesGen.AuthMethods;
  onSuspendUser: (user: TypesGen.User) => void;
  onDeleteUser: (user: TypesGen.User) => void;
  onListWorkspaces: (user: TypesGen.User) => void;
  onViewActivity: (user: TypesGen.User) => void;
  onActivateUser: (user: TypesGen.User) => void;
  onResetUserPassword: (user: TypesGen.User) => void;
  onUpdateUserRoles: (
    user: TypesGen.User,
    roles: TypesGen.Role["name"][],
  ) => void;
  filterProps: ComponentProps<typeof UsersFilter>;
  paginationRef: PaginationMachineRef;
  isNonInitialPage: boolean;
  actorID: string;
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
  oidcRoleSyncEnabled,
  canViewActivity,
  isLoading,
  filterProps,
  paginationRef,
  isNonInitialPage,
  actorID,
  authMethods,
}) => {
  return (
    <>
      <UsersFilter {...filterProps} />

      <TableToolbar>
        <PaginationStatus
          isLoading={Boolean(isLoading)}
          showing={users?.length ?? 0}
          total={count ?? 0}
          label="users"
        />
      </TableToolbar>

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
        oidcRoleSyncEnabled={oidcRoleSyncEnabled}
        canViewActivity={canViewActivity}
        isLoading={isLoading}
        isNonInitialPage={isNonInitialPage}
        actorID={actorID}
        authMethods={authMethods}
      />

      <PaginationWidget numRecords={count} paginationRef={paginationRef} />
    </>
  );
};
