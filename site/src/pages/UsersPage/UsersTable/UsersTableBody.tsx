import Box, { type BoxProps } from "@mui/material/Box";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import Skeleton from "@mui/material/Skeleton";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC } from "react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import type * as TypesGen from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Pill } from "components/Pill/Pill";
import { AvatarData } from "components/AvatarData/AvatarData";
import { AvatarDataSkeleton } from "components/AvatarData/AvatarDataSkeleton";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
  TableLoaderSkeleton,
  TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { TableRowMenu } from "components/TableRowMenu/TableRowMenu";
import { Stack } from "components/Stack/Stack";
import { EnterpriseBadge } from "components/DeploySettingsLayout/Badges";
import { EditRolesButton } from "./EditRolesButton";
import HideSourceOutlined from "@mui/icons-material/HideSourceOutlined";
import KeyOutlined from "@mui/icons-material/KeyOutlined";
import GitHub from "@mui/icons-material/GitHub";
import PasswordOutlined from "@mui/icons-material/PasswordOutlined";
import ShieldOutlined from "@mui/icons-material/ShieldOutlined";

dayjs.extend(relativeTime);

const isOwnerRole = (role: TypesGen.Role): boolean => {
  return role.name === "owner";
};

const roleOrder = ["owner", "user-admin", "template-admin", "auditor"];

const sortRoles = (roles: TypesGen.Role[]) => {
  return roles.slice(0).sort((a, b) => {
    return roleOrder.indexOf(a.name) - roleOrder.indexOf(b.name);
  });
};

interface UsersTableBodyProps {
  users?: TypesGen.User[];
  authMethods?: TypesGen.AuthMethods;
  roles?: TypesGen.AssignableRoles[];
  isUpdatingUserRoles?: boolean;
  canEditUsers?: boolean;
  isLoading?: boolean;
  canViewActivity?: boolean;
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
  isNonInitialPage: boolean;
  actorID: string;
  // oidcRoleSyncEnabled should be set to false if unknown.
  // This is used to determine if the oidc roles are synced from the oidc idp and
  // editing via the UI should be disabled.
  oidcRoleSyncEnabled: boolean;
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
  return (
    <ChooseOne>
      <Cond condition={Boolean(isLoading)}>
        <TableLoaderSkeleton>
          <TableRowSkeleton>
            <TableCell>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <AvatarDataSkeleton />
              </Box>
            </TableCell>
            <TableCell>
              <Skeleton variant="text" width="25%" />
            </TableCell>
            <TableCell>
              <Skeleton variant="text" width="25%" />
            </TableCell>
            <TableCell>
              <Skeleton variant="text" width="25%" />
            </TableCell>
            {canEditUsers && (
              <TableCell>
                <Skeleton variant="text" width="25%" />
              </TableCell>
            )}
          </TableRowSkeleton>
        </TableLoaderSkeleton>
      </Cond>
      <Cond condition={!users || users.length === 0}>
        <ChooseOne>
          <Cond condition={isNonInitialPage}>
            <TableRow>
              <TableCell colSpan={999}>
                <Box p={4}>
                  <EmptyState message="No users found on this page" />
                </Box>
              </TableCell>
            </TableRow>
          </Cond>
          <Cond>
            <TableRow>
              <TableCell colSpan={999}>
                <Box p={4}>
                  <EmptyState message="No users found" />
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
              };
              const userRoles =
                user.roles.length === 0
                  ? [fallbackRole]
                  : sortRoles(user.roles);

              return (
                <TableRow key={user.id} data-testid={`user-${user.id}`}>
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
                            );
                            onUpdateUserRoles(user, rolesWithoutFallback);
                          }}
                        />
                      )}
                      {userRoles.map((role) => (
                        <Pill
                          key={role.name}
                          text={role.display_name}
                          css={[
                            styles.rolePill,
                            isOwnerRole(role) && styles.rolePillOwner,
                          ]}
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
                    css={[
                      styles.status,
                      user.status === "suspended" && styles.suspended,
                    ]}
                  >
                    <Box>{user.status}</Box>
                    <LastSeen value={user.last_seen_at} sx={{ fontSize: 12 }} />
                  </TableCell>

                  {canEditUsers && (
                    <TableCell>
                      <TableRowMenu
                        data={user}
                        menuItems={[
                          // Return either suspend or activate depending on status
                          user.status === "active" || user.status === "dormant"
                            ? {
                                label: <>Suspend&hellip;</>,
                                onClick: onSuspendUser,
                                disabled: false,
                              }
                            : {
                                label: <>Activate&hellip;</>,
                                onClick: onActivateUser,
                                disabled: false,
                              },
                          {
                            label: <>Delete&hellip;</>,
                            onClick: onDeleteUser,
                            disabled: user.id === actorID,
                          },
                          {
                            label: <>Reset password&hellip;</>,
                            onClick: onResetUserPassword,
                            disabled: user.login_type !== "password",
                          },
                          {
                            label: "View workspaces",
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
                        ]}
                      />
                    </TableCell>
                  )}
                </TableRow>
              );
            })}
        </>
      </Cond>
    </ChooseOne>
  );
};

const LoginType = ({
  authMethods,
  value,
}: {
  authMethods: TypesGen.AuthMethods;
  value: TypesGen.LoginType;
}) => {
  let displayName: string = value;
  let icon = <></>;
  const iconStyles = { width: 14, height: 14 };

  if (value === "password") {
    displayName = "Password";
    icon = <PasswordOutlined sx={iconStyles} />;
  } else if (value === "none") {
    displayName = "None";
    icon = <HideSourceOutlined sx={iconStyles} />;
  } else if (value === "github") {
    displayName = "GitHub";
    icon = <GitHub sx={iconStyles} />;
  } else if (value === "token") {
    displayName = "Token";
    icon = <KeyOutlined sx={iconStyles} />;
  } else if (value === "oidc") {
    displayName =
      authMethods.oidc.signInText === "" ? "OIDC" : authMethods.oidc.signInText;
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
      );
  }

  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, fontSize: 14 }}>
      {icon}
      {displayName}
    </Box>
  );
};

const LastSeen = ({ value, ...boxProps }: { value: string } & BoxProps) => {
  const theme = useTheme();
  const t = dayjs(value);
  const now = dayjs();

  let message = t.fromNow();
  let color = theme.palette.text.secondary;

  if (t.isAfter(now.subtract(1, "hour"))) {
    color = theme.palette.success.light;
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now";
  } else if (t.isAfter(now.subtract(3, "day"))) {
    color = theme.palette.text.secondary;
  } else if (t.isAfter(now.subtract(1, "month"))) {
    color = theme.palette.warning.light;
  } else if (t.isAfter(now.subtract(100, "year"))) {
    color = theme.palette.error.light;
  } else {
    message = "Never";
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
  );
};

const styles = {
  status: {
    textTransform: "capitalize",
  },
  suspended: (theme) => ({
    color: theme.palette.text.secondary,
  }),
  rolePill: (theme) => ({
    backgroundColor: theme.palette.background.paperLight,
    borderColor: theme.palette.divider,
  }),
  rolePillOwner: (theme) => ({
    backgroundColor: theme.palette.info.dark,
    borderColor: theme.palette.info.light,
  }),
} satisfies Record<string, Interpolation<Theme>>;
