import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { combineClasses } from "../../util/combineClasses"
import { AvatarData } from "../AvatarData/AvatarData"
import { EmptyState } from "../EmptyState/EmptyState"
import { RoleSelect } from "../RoleSelect/RoleSelect"
import { TableLoader } from "../TableLoader/TableLoader"
import { TableRowMenu } from "../TableRowMenu/TableRowMenu"

export const Language = {
  emptyMessage: "No users found",
  suspendMenuItem: "Suspend",
  activateMenuItem: "Activate",
  resetPasswordMenuItem: "Reset password",
}

interface UsersTableBodyProps {
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

export const UsersTableBody: FC<React.PropsWithChildren<UsersTableBodyProps>> = ({
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
  const styles = useStyles()

  if (isLoading) {
    return <TableLoader />
  }

  if (!users || !users.length) {
    return (
      <TableRow>
        <TableCell colSpan={999}>
          <Box p={4}>
            <EmptyState message={Language.emptyMessage} />
          </Box>
        </TableCell>
      </TableRow>
    )
  }

  return (
    <>
      {users.map((user) => {
        // When the user has no role we want to show they are a Member
        const fallbackRole: TypesGen.Role = {
          name: "member",
          display_name: "Member",
        }
        const userRoles = user.roles.length === 0 ? [fallbackRole] : user.roles

        return (
          <TableRow key={user.id}>
            <TableCell>
              <AvatarData title={user.username} subtitle={user.email} highlightTitle />
            </TableCell>
            <TableCell
              className={combineClasses([
                styles.status,
                user.status === "suspended" ? styles.suspended : undefined,
              ])}
            >
              {user.status}
            </TableCell>
            <TableCell>
              {canEditUsers ? (
                <RoleSelect
                  roles={roles ?? []}
                  selectedRoles={userRoles}
                  loading={isUpdatingUserRoles}
                  onChange={(roles) => {
                    // Remove the fallback role because it is only for the UI
                    roles = roles.filter((role) => role !== fallbackRole.name)
                    onUpdateUserRoles(user, roles)
                  }}
                />
              ) : (
                <>{userRoles.map((role) => role.display_name).join(", ")}</>
              )}
            </TableCell>
            {canEditUsers && (
              <TableCell>
                <TableRowMenu
                  data={user}
                  menuItems={
                    // Return either suspend or activate depending on status
                    (user.status === "active"
                      ? [
                          {
                            label: Language.suspendMenuItem,
                            onClick: onSuspendUser,
                          },
                        ]
                      : [
                          {
                            label: Language.activateMenuItem,
                            onClick: onActivateUser,
                          },
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
        )
      })}
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  status: {
    textTransform: "capitalize",
  },
  suspended: {
    color: theme.palette.text.secondary,
  },
}))
