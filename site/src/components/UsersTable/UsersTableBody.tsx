import Box, { BoxProps } from "@mui/material/Box"
import { makeStyles, useTheme } from "@mui/styles"
import TableCell from "@mui/material/TableCell"
import TableRow from "@mui/material/TableRow"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Pill } from "components/Pill/Pill"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import * as TypesGen from "../../api/typesGenerated"
import { combineClasses } from "../../utils/combineClasses"
import { AvatarData } from "../AvatarData/AvatarData"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoaderSkeleton } from "../TableLoader/TableLoader"
import { TableRowMenu } from "../TableRowMenu/TableRowMenu"
import { EditRolesButton } from "components/EditRolesButton/EditRolesButton"
import { Stack } from "components/Stack/Stack"
import { EnterpriseBadge } from "components/DeploySettingsLayout/Badges"
import dayjs from "dayjs"
import { SxProps, Theme } from "@mui/material/styles"
import HideSourceOutlined from "@mui/icons-material/HideSourceOutlined"
import KeyOutlined from "@mui/icons-material/KeyOutlined"
import GitHub from "@mui/icons-material/GitHub"
import PasswordOutlined from "@mui/icons-material/PasswordOutlined"
import relativeTime from "dayjs/plugin/relativeTime"
import ShieldOutlined from "@mui/icons-material/ShieldOutlined"

dayjs.extend(relativeTime)

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
  authMethods?: TypesGen.AuthMethods
  roles?: TypesGen.AssignableRoles[]
  isUpdatingUserRoles?: boolean
  canEditUsers?: boolean
  isLoading?: boolean
  canViewActivity?: boolean
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
  isNonInitialPage: boolean
  actorID: string
  // oidcRoleSyncEnabled should be set to false if unknown.
  // This is used to determine if the oidc roles are synced from the oidc idp and
  // editing via the UI should be disabled.
  oidcRoleSyncEnabled: boolean
}

export const UsersTableBody: FC<
  React.PropsWithChildren<UsersTableBodyProps>
> = ({
  users,
  authMethods,
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
}) => {
  const styles = useStyles()
  const { t } = useTranslation("usersPage")

  return (
    <ChooseOne>
      <Cond condition={Boolean(isLoading)}>
        <TableLoaderSkeleton columns={canEditUsers ? 5 : 4} useAvatarData />
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
                          userLoginType={user.login_type}
                          oidcRoleSync={oidcRoleSyncEnabled}
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
                  <TableCell>
                    <LoginType
                      authMethods={authMethods!}
                      value={user.login_type}
                    />
                  </TableCell>
                  <TableCell
                    className={combineClasses([
                      styles.status,
                      user.status === "suspended"
                        ? styles.suspended
                        : undefined,
                    ])}
                  >
                    <Box>{user.status}</Box>
                    <LastSeen value={user.last_seen_at} sx={{ fontSize: 12 }} />
                  </TableCell>

                  {canEditUsers && (
                    <TableCell>
                      <TableRowMenu
                        data={user}
                        menuItems={
                          // Return either suspend or activate depending on status
                          (user.status === "active" || user.status === "dormant"
                            ? [
                                {
                                  label: t(
                                    "suspendMenuItem",
                                  ) as React.ReactNode,
                                  onClick: onSuspendUser,
                                  disabled: false,
                                },
                              ]
                            : [
                                {
                                  label: t(
                                    "activateMenuItem",
                                  ) as React.ReactNode,
                                  onClick: onActivateUser,
                                  disabled: false,
                                },
                              ]
                          ).concat(
                            {
                              label: t("deleteMenuItem"),
                              onClick: onDeleteUser,
                              disabled: user.id === actorID,
                            },
                            {
                              label: t("resetPasswordMenuItem"),
                              onClick: onResetUserPassword,
                              disabled: user.login_type !== "password",
                            },
                            {
                              label: t("listWorkspacesMenuItem"),
                              onClick: onListWorkspaces,
                              disabled: false,
                            },
                            {
                              label: (
                                <>
                                  View activity
                                  {!canViewActivity && <EnterpriseBadge />}
                                </>
                              ),
                              onClick: onViewActivity,
                              disabled: !canViewActivity,
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

const LoginType = ({
  authMethods,
  value,
}: {
  authMethods: TypesGen.AuthMethods
  value: TypesGen.LoginType
}) => {
  let displayName = value as string
  let icon = <></>
  const iconStyles: SxProps = { width: 14, height: 14 }

  if (value === "password") {
    displayName = "Password"
    icon = <PasswordOutlined sx={iconStyles} />
  } else if (value === "none") {
    displayName = "None"
    icon = <HideSourceOutlined sx={iconStyles} />
  } else if (value === "github") {
    displayName = "GitHub"
    icon = <GitHub sx={iconStyles} />
  } else if (value === "token") {
    displayName = "Token"
    icon = <KeyOutlined sx={iconStyles} />
  } else if (value === "oidc") {
    displayName =
      authMethods.oidc.signInText === "" ? "OIDC" : authMethods.oidc.signInText
    icon =
      authMethods.oidc.iconUrl === "" ? (
        <ShieldOutlined sx={iconStyles} />
      ) : (
        <Box
          component="img"
          alt="Open ID Connect icon"
          src={authMethods.oidc.iconUrl}
          sx={iconStyles}
        />
      )
  }

  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, fontSize: 14 }}>
      {icon}
      {displayName}
    </Box>
  )
}

const LastSeen = ({ value, ...boxProps }: { value: string } & BoxProps) => {
  const theme: Theme = useTheme()
  const t = dayjs(value)
  const now = dayjs()

  let message = t.fromNow()
  let color = theme.palette.text.secondary

  if (t.isAfter(now.subtract(1, "hour"))) {
    color = theme.palette.success.light
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now"
  } else if (t.isAfter(now.subtract(3, "day"))) {
    color = theme.palette.text.secondary
  } else if (t.isAfter(now.subtract(1, "month"))) {
    color = theme.palette.warning.light
  } else if (t.isAfter(now.subtract(100, "year"))) {
    color = theme.palette.error.light
  } else {
    message = "Never"
  }

  return (
    <Box
      component="span"
      data-chromatic="ignore"
      {...boxProps}
      sx={{ color, ...boxProps.sx }}
    >
      {message}
    </Box>
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
