import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { UsersTableBody } from "./UsersTableBody"

export const Language = {
  usernameLabel: "User",
  rolesLabel: "Roles",
  statusLabel: "Status",
}

export interface UsersTableProps {
  users?: TypesGen.User[]
  roles?: TypesGen.Role[]
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  isLoading?: boolean
  onSuspendUser: (user: TypesGen.User) => void
  onActivateUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (user: TypesGen.User, roles: TypesGen.Role["name"][]) => void
}

export const UsersTable: FC<React.PropsWithChildren<UsersTableProps>> = ({
  users,
  roles,
  onSuspendUser,
  onActivateUser,
  onResetUserPassword,
  onUpdateUserRoles,
  isUpdatingUserRoles,
  canEditUsers,
  isLoading,
}) => {
  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>{Language.usernameLabel}</TableCell>
            <TableCell>{Language.statusLabel}</TableCell>
            <TableCell>{Language.rolesLabel}</TableCell>
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
            onResetUserPassword={onResetUserPassword}
            onSuspendUser={onSuspendUser}
            onUpdateUserRoles={onUpdateUserRoles}
          />
        </TableBody>
      </Table>
    </TableContainer>
  )
}
