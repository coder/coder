import Box from "@material-ui/core/Box"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { EmptyState } from "../EmptyState/EmptyState"
import { RoleSelect } from "../RoleSelect/RoleSelect"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"
import { TableLoader } from "../TableLoader/TableLoader"
import { TableRowMenu } from "../TableRowMenu/TableRowMenu"
import { TableTitle } from "../TableTitle/TableTitle"
import { UserCell } from "../UserCell/UserCell"

export const Language = {
  pageTitle: "Users",
  usersTitle: "All users",
  emptyMessage: "No users found",
  usernameLabel: "User",
  suspendMenuItem: "Suspend",
  resetPasswordMenuItem: "Reset password",
  rolesLabel: "Roles",
}

export interface UsersTableProps {
  users?: TypesGen.User[]
  roles?: TypesGen.Role[]
  isUpdatingUserRoles?: boolean
  onSuspendUser: (user: TypesGen.User) => void
  onResetUserPassword: (user: TypesGen.User) => void
  onUpdateUserRoles: (user: TypesGen.User, roles: TypesGen.Role["name"][]) => void
}

export const UsersTable: React.FC<UsersTableProps> = ({
  users,
  roles,
  onSuspendUser,
  onResetUserPassword,
  onUpdateUserRoles,
  isUpdatingUserRoles,
}) => {
  const isLoading = !users || !roles

  return (
    <Table>
      <TableHead>
        <TableTitle title={Language.usersTitle} />
        <TableHeaderRow>
          <TableCell size="small">{Language.usernameLabel}</TableCell>
          <TableCell size="small">{Language.rolesLabel}</TableCell>
          {/* 1% is a trick to make the table cell width fit the content */}
          <TableCell size="small" width="1%" />
        </TableHeaderRow>
      </TableHead>
      <TableBody>
        {isLoading && <TableLoader />}
        {users &&
          roles &&
          users.map((u) => (
            <TableRow key={u.id}>
              <TableCell>
                <UserCell Avatar={{ username: u.username }} primaryText={u.username} caption={u.email} />{" "}
              </TableCell>
              <TableCell>
                <RoleSelect
                  roles={roles}
                  selectedRoles={u.roles}
                  loading={isUpdatingUserRoles}
                  onChange={(roles) => onUpdateUserRoles(u, roles)}
                />
              </TableCell>
              <TableCell>
                <TableRowMenu
                  data={u}
                  menuItems={[
                    {
                      label: Language.suspendMenuItem,
                      onClick: onSuspendUser,
                    },
                    {
                      label: Language.resetPasswordMenuItem,
                      onClick: onResetUserPassword,
                    },
                  ]}
                />
              </TableCell>
            </TableRow>
          ))}

        {users && users.length === 0 && (
          <TableRow>
            <TableCell colSpan={999}>
              <Box p={4}>
                <EmptyState message={Language.emptyMessage} />
              </Box>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  )
}
