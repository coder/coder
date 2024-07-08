import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { FC } from "react";
import type { GroupsByUserId } from "api/queries/groups";
import type * as TypesGen from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { TableColumnHelpTooltip } from "./TableColumnHelpTooltip";
import { UsersTableBody } from "./UsersTableBody";

export const Language = {
  usernameLabel: "User",
  rolesLabel: "Roles",
  groupsLabel: "Groups",
  statusLabel: "Status",
  lastSeenLabel: "Last Seen",
  loginTypeLabel: "Login Type",
} as const;

export interface UsersTableProps {
  users: readonly TypesGen.User[] | undefined;
  roles: TypesGen.AssignableRoles[] | undefined;
  groupsByUserId: GroupsByUserId | undefined;
  isUpdatingUserRoles?: boolean;
  canEditUsers: boolean;
  canViewActivity?: boolean;
  isLoading: boolean;
  onSuspendUser: (user: TypesGen.User) => void;
  onActivateUser: (user: TypesGen.User) => void;
  onDeleteUser: (user: TypesGen.User) => void;
  onListWorkspaces: (user: TypesGen.User) => void;
  onViewActivity: (user: TypesGen.User) => void;
  onResetUserPassword: (user: TypesGen.User) => void;
  onUpdateUserRoles: (
    user: TypesGen.User,
    roles: TypesGen.SlimRole["name"][],
  ) => void;
  isNonInitialPage: boolean;
  actorID: string;
  oidcRoleSyncEnabled: boolean;
  authMethods?: TypesGen.AuthMethods;
}

export const UsersTable: FC<UsersTableProps> = ({
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
  canViewActivity,
  isLoading,
  isNonInitialPage,
  actorID,
  oidcRoleSyncEnabled,
  authMethods,
  groupsByUserId,
}) => {
  return (
    <TableContainer>
      <Table data-testid="users-table">
        <TableHead>
          <TableRow>
            <TableCell width="29%">{Language.usernameLabel}</TableCell>

            <TableCell width="29%">
              <Stack direction="row" spacing={1} alignItems="center">
                <span>{Language.rolesLabel}</span>
                <TableColumnHelpTooltip variant="roles" />
              </Stack>
            </TableCell>

            <TableCell width="14%">
              <Stack direction="row" spacing={1} alignItems="center">
                <span>{Language.groupsLabel}</span>
                <TableColumnHelpTooltip variant="groups" />
              </Stack>
            </TableCell>

            <TableCell width="14%">{Language.loginTypeLabel}</TableCell>
            <TableCell width="14%">{Language.statusLabel}</TableCell>

            {/* 1% is a trick to make the table cell width fit the content */}
            {canEditUsers && <TableCell width="1%" />}
          </TableRow>
        </TableHead>

        <TableBody>
          <UsersTableBody
            users={users}
            roles={roles}
            groupsByUserId={groupsByUserId}
            isLoading={isLoading}
            canEditUsers={canEditUsers}
            canViewActivity={canViewActivity}
            isUpdatingUserRoles={isUpdatingUserRoles}
            onActivateUser={onActivateUser}
            onDeleteUser={onDeleteUser}
            onListWorkspaces={onListWorkspaces}
            onViewActivity={onViewActivity}
            onResetUserPassword={onResetUserPassword}
            onSuspendUser={onSuspendUser}
            onUpdateUserRoles={onUpdateUserRoles}
            isNonInitialPage={isNonInitialPage}
            actorID={actorID}
            oidcRoleSyncEnabled={oidcRoleSyncEnabled}
            authMethods={authMethods}
          />
        </TableBody>
      </Table>
    </TableContainer>
  );
};
