import { useState } from "react";
import { useTheme } from "@emotion/react";
import { type User, type Role } from "api/typesGenerated";

import { EditRolesButton } from "./EditRolesButton";
import { Pill } from "components/Pill/Pill";
import TableCell from "@mui/material/TableCell";
import Stack from "@mui/material/Stack";

const roleNameDisplayOrder: readonly string[] = [
  "owner",
  "user-admin",
  "template-admin",
  "auditor",
];

const fallbackRole: Role = {
  name: "member",
  display_name: "Member",
} as const;

function getPillRoleList(userRoles: readonly Role[]): readonly Role[] {
  if (userRoles.length === 0) {
    return [fallbackRole];
  }

  const matchedOwnerRole = userRoles.find((role) => role.name === "owner");
  if (matchedOwnerRole !== undefined) {
    return [matchedOwnerRole];
  }

  return [...userRoles].sort((r1, r2) => {
    if (r1.name === r2.name) {
      return 0;
    }

    return r1.name < r2.name ? -1 : 1;
  });
}

function getSelectedRoleNames(roles: readonly Role[]) {
  const roleNameSet = new Set(roles.map((role) => role.name));
  if (roleNameSet.size === 0) {
    roleNameSet.add(fallbackRole.name);
  }

  return roleNameSet;
}

function sortRolesByAccessLevel(roles: readonly Role[]) {
  return [...roles].sort(
    (r1, r2) =>
      roleNameDisplayOrder.indexOf(r1.name) -
      roleNameDisplayOrder.indexOf(r2.name),
  );
}

type Props = {
  canEditUsers: boolean;
  roles: undefined | readonly Role[];
  user: User;
  isLoading: boolean;
  oidcRoleSyncEnabled: boolean;
  onUserRolesUpdate: (user: User, newRoleNames: string[]) => void;
};

export function UserRoleCell({
  canEditUsers,
  roles,
  user,
  isLoading,
  oidcRoleSyncEnabled,
  onUserRolesUpdate,
}: Props) {
  const theme = useTheme();

  const pillRoleList = getPillRoleList(user.roles);
  const [rolesTruncated, setRolesTruncated] = useState(
    user.roles.length - pillRoleList.length,
  );

  return (
    <TableCell>
      <Stack direction="row" spacing={1}>
        {canEditUsers && (
          <EditRolesButton
            roles={sortRolesByAccessLevel(roles ?? [])}
            selectedRoleNames={getSelectedRoleNames(user.roles)}
            isLoading={isLoading}
            userLoginType={user.login_type}
            oidcRoleSync={oidcRoleSyncEnabled}
            onChange={(roles) => {
              // Remove the fallback role because it is only for the UI
              const rolesWithoutFallback = roles.filter(
                (role) => role !== fallbackRole.name,
              );

              onUserRolesUpdate(user, rolesWithoutFallback);
            }}
          />
        )}

        {pillRoleList.map((role) => {
          const isOwnerRole = role.name === "owner";
          const { palette } = theme;

          return (
            <Pill
              key={role.name}
              text={role.display_name}
              css={{
                backgroundColor: isOwnerRole
                  ? palette.info.dark
                  : palette.background.paperLight,
                borderColor: isOwnerRole ? palette.info.light : palette.divider,
              }}
            />
          );
        })}
      </Stack>
    </TableCell>
  );
}
