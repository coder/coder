import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableHead from "@mui/material/TableHead"
import TableRow from "@mui/material/TableRow"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { Stack } from "../Stack/Stack"
import { UserRoleHelpTooltip } from "../Tooltips"
import { UsersTableBody } from "./UsersTableBody"

export const Language = {
  usernameLabel: "User",
  rolesLabel: "Roles",
  statusLabel: "Status",
  lastSeenLabel: "Last Seen",
}

export interface UsersTableProps {
  users?: TypesGen.User[]
  roles?: TypesGen.AssignableRoles[]
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  isLoading?: boolean
  onSuspendUser: (user: TypesGen.User) => void
  onActivateUser: (user: TypesGen.User) => void
  onDeleteUser: (user: TypesGen.User) => void
  onListWorkspaces: (user: TypesGen.User) => void
  onViewActivity: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (
    user: TypesGen.User,
    roles: TypesGen.Role["name"][],
  ) => void
  isNonInitialPage: boolean
  actorID: string
}

export const UsersTable: FC<React.PropsWithChildren<UsersTableProps>> = ({
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
  isLoading,
  isNonInitialPage,
  actorID,
}) => {
  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width="30%">{Language.usernameLabel}</TableCell>
            <TableCell width="40%">
              <Stack direction="row" spacing={1} alignItems="center">
                <span>{Language.rolesLabel}</span>
                <UserRoleHelpTooltip />
              </Stack>
            </TableCell>
            <TableCell width="15%">{Language.statusLabel}</TableCell>
            <TableCell width="15%">{Language.lastSeenLabel}</TableCell>

            {/* 1% is a trick to make the table cell width fit the content */}
            {canEditUsers && <TableCell width="1%" />}
          </TableRow>
        </TableHead>
        <TableBody>
          <UsersTableBody
            users={users}
            roles={roles}
            isLoading={isLoading}
            canEditUsers={canEditUsers}
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
          />
        </TableBody>
      </Table>
    </TableContainer>
  )
}
