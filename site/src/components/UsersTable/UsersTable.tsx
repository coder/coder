import Box from "@material-ui/core/Box"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { AvatarData } from "../AvatarData/AvatarData"
import { EmptyState } from "../EmptyState/EmptyState"
import { RoleSelect } from "../RoleSelect/RoleSelect"
import { TableLoader } from "../TableLoader/TableLoader"
import { TableRowMenu } from "../TableRowMenu/TableRowMenu"

export const Language = {
  pageTitle: "Users",
  usersTitle: "All users",
  emptyMessage: "No users found",
  usernameLabel: "User",
  suspendMenuItem: "Suspend",
  activateMenuItem: "Activate",
  resetPasswordMenuItem: "Reset password",
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
  canEditUsers,
  isLoading,
}) => {
  return (
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
        {isLoading && <TableLoader />}
        {!isLoading &&
          users &&
          users.map((u) => (
            <TableRow key={u.id}>
              <TableCell>
                <AvatarData title={u.username} subtitle={u.email} />
              </TableCell>
              <TableCell>{u.status}</TableCell>
              <TableCell>
                {canEditUsers ? (
                  <RoleSelect
                    roles={roles ?? []}
                    selectedRoles={u.roles}
                    loading={isUpdatingUserRoles}
                    onChange={(roles) => onUpdateUserRoles(u, roles)}
                  />
                ) : (
                  <>{u.roles.map((r) => r.display_name).join(", ")}</>
                )}
              </TableCell>
              {canEditUsers && (
                <TableCell>
                  <TableRowMenu
                    data={u}
                    menuItems={
                      // Return either suspend or activate depending on status
                      (u.status === "active"
                        ? [
                            {
                              label: Language.suspendMenuItem,
                              onClick: onSuspendUser,
                            },
                          ]
                        : [
                            // TODO: Uncomment this and add activate user functionality.
                            // {
                            //   label: Language.activateMenuItem,
                            //   // eslint-disable-next-line @typescript-eslint/no-empty-function
                            //   onClick: function () {},
                            // },
                          ]
                      ).concat({
                        label: Language.resetPasswordMenuItem,
                        onClick: onResetUserPassword,
                      })
                    }
                  />
                </TableCell>
              )}
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
