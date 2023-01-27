import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { LastUsed } from "components/LastUsed/LastUsed"
import { Pill } from "components/Pill/Pill"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import * as TypesGen from "../../api/typesGenerated"
import { combineClasses } from "../../util/combineClasses"
import { AvatarData } from "../AvatarData/AvatarData"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoader } from "../TableLoader/TableLoader"
import { TableRowMenu } from "../TableRowMenu/TableRowMenu"
import { EditRolesButton } from "components/EditRolesButton/EditRolesButton"
import { Stack } from "components/Stack/Stack"

const isOwnerRole = (role: TypesGen.Role): boolean => {
  return role.name === "owner"
}

const roleOrder = ["owner", "user-admin", "template-admin", "auditor"]

const sortRoles = (roles: TypesGen.Role[]) => {
  return roles.slice(0).sort((a, b) => {
    return roleOrder.indexOf(a.name) - roleOrder.indexOf(b.name)
  })
}

interface UsersTableBodyProps {
  users?: TypesGen.User[]
  roles?: TypesGen.AssignableRoles[]
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
  isNonInitialPage: boolean
}

export const UsersTableBody: FC<
  React.PropsWithChildren<UsersTableBodyProps>
> = ({
  users,
  roles,
  onSuspendUser,
  onDeleteUser,
  onListWorkspaces,
  onActivateUser,
  onResetUserPassword,
  onUpdateUserRoles,
  isUpdatingUserRoles,
  canEditUsers,
  isLoading,
  isNonInitialPage,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("usersPage")

  return (
    <ChooseOne>
      <Cond condition={Boolean(isLoading)}>
        <TableLoader />
      </Cond>
      <Cond condition={!users || users.length === 0}>
        <ChooseOne>
          <Cond condition={isNonInitialPage}>
            <TableRow>
              <TableCell colSpan={999}>
                <Box p={4}>
                  <EmptyState message={t("emptyPageMessage")} />
                </Box>
              </TableCell>
            </TableRow>
          </Cond>
          <Cond>
            <TableRow>
              <TableCell colSpan={999}>
                <Box p={4}>
                  <EmptyState message={t("emptyMessage")} />
                </Box>
              </TableCell>
            </TableRow>
          </Cond>
        </ChooseOne>
      </Cond>
      <Cond>
        <>
          {users &&
            users.map((user) => {
              // When the user has no role we want to show they are a Member
              const fallbackRole: TypesGen.Role = {
                name: "member",
                display_name: "Member",
              }
              const userRoles =
                user.roles.length === 0 ? [fallbackRole] : sortRoles(user.roles)

              return (
                <TableRow key={user.id}>
                  <TableCell>
                    <AvatarData
                      title={user.username}
                      subtitle={user.email}
                      src={user.avatar_url}
                    />
                  </TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={1}>
                      {canEditUsers && (
                        <EditRolesButton
                          roles={roles ? sortRoles(roles) : []}
                          selectedRoles={userRoles}
                          isLoading={Boolean(isUpdatingUserRoles)}
                          onChange={(roles) => {
                            // Remove the fallback role because it is only for the UI
                            const rolesWithoutFallback = roles.filter(
                              (role) => role !== fallbackRole.name,
                            )
                            onUpdateUserRoles(user, rolesWithoutFallback)
                          }}
                        />
                      )}
                      {userRoles.map((role) => (
                        <Pill
                          key={role.name}
                          text={role.display_name}
                          className={combineClasses({
                            [styles.rolePill]: true,
                            [styles.rolePillOwner]: isOwnerRole(role),
                          })}
                        />
                      ))}
                    </Stack>
                  </TableCell>
                  <TableCell
                    className={combineClasses([
                      styles.status,
                      user.status === "suspended"
                        ? styles.suspended
                        : undefined,
                    ])}
                  >
                    {user.status}
                  </TableCell>
                  <TableCell>
                    <LastUsed lastUsedAt={user.last_seen_at} />
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
                                  label: t("suspendMenuItem"),
                                  onClick: onSuspendUser,
                                },
                              ]
                            : [
                                {
                                  label: t("activateMenuItem"),
                                  onClick: onActivateUser,
                                },
                              ]
                          ).concat(
                            {
                              label: t("deleteMenuItem"),
                              onClick: onDeleteUser,
                            },
                            {
                              label: t("listWorkspacesMenuItem"),
                              onClick: onListWorkspaces,
                            },
                            {
                              label: t("resetPasswordMenuItem"),
                              onClick: onResetUserPassword,
                            },
                          )
                        }
                      />
                    </TableCell>
                  )}
                </TableRow>
              )
            })}
        </>
      </Cond>
    </ChooseOne>
  )
}

const useStyles = makeStyles((theme) => ({
  status: {
    textTransform: "capitalize",
  },
  suspended: {
    color: theme.palette.text.secondary,
  },
  rolePill: {
    backgroundColor: theme.palette.background.paperLight,
    borderColor: theme.palette.divider,
  },
  rolePillOwner: {
    backgroundColor: theme.palette.info.dark,
    borderColor: theme.palette.info.light,
  },
}))
