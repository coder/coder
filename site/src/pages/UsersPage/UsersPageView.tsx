import { type ComponentProps, type FC } from "react";
import type * as TypesGen from "api/typesGenerated";
import { type GroupsByUserId } from "api/queries/groups";
import {
  PaginationContainer,
  type PaginationResult,
} from "components/PaginationWidget/PaginationContainer";
import { UsersTable } from "./UsersTable/UsersTable";
import { UsersFilter } from "./UsersFilter";

export interface UsersPageViewProps {
  users?: TypesGen.User[];
  roles?: TypesGen.AssignableRoles[];
  isUpdatingUserRoles?: boolean;
  canEditUsers: boolean;
  oidcRoleSyncEnabled: boolean;
  canViewActivity?: boolean;
  isLoading: boolean;
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
  isNonInitialPage: boolean;
  actorID: string;
  groupsByUserId: GroupsByUserId | undefined;
  usersQuery: PaginationResult;
}

export const UsersPageView: FC<UsersPageViewProps> = ({
  users,
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
  isNonInitialPage,
  actorID,
  authMethods,
  groupsByUserId,
  usersQuery,
}) => {
  return (
    <>
      <UsersFilter {...filterProps} />

      <PaginationContainer query={usersQuery} paginationUnitLabel="users">
        <UsersTable
          users={users}
          roles={roles}
          groupsByUserId={groupsByUserId}
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
      </PaginationContainer>
    </>
  );
};
