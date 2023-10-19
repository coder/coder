/**
 * @file Defines the visual logic for the Roles cell in the Users page table.
 *
 * The previous implementation tried to dynamically truncate the number of roles
 * that would get displayed in a cell, only truncating if there were more roles
 * than room in the cell. But there was a problem – that information can't
 * exist on the first render, because the DOM nodes haven't been made yet.
 *
 * The only way to avoid UI flickering was by juggling between useLayoutEffect
 * for direct DOM node mutations for any renders that had new data, and normal
 * state logic for all other renders. It was clunky, and required duplicating
 * the logic in two places (making things easy to accidentally break), so we
 * went with a simpler design. If we decide we really do need to display the
 * users like that, though, know that it will be painful
 */
import { useTheme } from "@emotion/react";
import { type User, type Role } from "api/typesGenerated";

import { EditRolesButton } from "./EditRolesButton";
import { Pill } from "components/Pill/Pill";
import TableCell from "@mui/material/TableCell";
import Stack from "@mui/material/Stack";

import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "components/Popover/Popover";

type UserRoleCellProps = {
  canEditUsers: boolean;
  allAvailableRoles: Role[] | undefined;
  user: User;
  isLoading: boolean;
  oidcRoleSyncEnabled: boolean;
  onUserRolesUpdate: (user: User, newRoleNames: string[]) => void;
};

export function UserRoleCell({
  canEditUsers,
  allAvailableRoles,
  user,
  isLoading,
  oidcRoleSyncEnabled,
  onUserRolesUpdate,
}: UserRoleCellProps) {
  const theme = useTheme();

  const [mainDisplayRole = fallbackRole, ...extraRoles] =
    sortRolesByAccessLevel(user.roles ?? []);
  const hasOwnerRole = mainDisplayRole.name === "owner";

  return (
    <TableCell>
      <Stack direction="row" spacing={1}>
        {canEditUsers && (
          <EditRolesButton
            roles={sortRolesByAccessLevel(allAvailableRoles ?? [])}
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

        <Pill
          text={mainDisplayRole.display_name}
          css={{
            backgroundColor: hasOwnerRole
              ? theme.palette.info.dark
              : theme.palette.background.paperLight,
            borderColor: hasOwnerRole
              ? theme.palette.info.light
              : theme.palette.divider,
          }}
        />

        {extraRoles.length > 0 && <OverflowRolePill roles={extraRoles} />}
      </Stack>
    </TableCell>
  );
}

type OverflowRolePillProps = {
  roles: readonly Role[];
};

function OverflowRolePill({ roles }: OverflowRolePillProps) {
  const theme = useTheme();

  return (
    <Popover mode="hover">
      <PopoverTrigger>
        <Pill
          text={`+${roles.length} more`}
          css={{
            backgroundColor: theme.palette.background.paperLight,
            borderColor: theme.palette.divider,
          }}
        />
      </PopoverTrigger>

      <PopoverContent
        disableRestoreFocus
        disableScrollLock
        css={{
          ".MuiPaper-root": {
            display: "flex",
            flexFlow: "row wrap",
            columnGap: theme.spacing(1),
            rowGap: theme.spacing(1.5),
            padding: theme.spacing(1.5, 2),
            alignContent: "space-around",
          },
        }}
      >
        {roles.map((role) => (
          <Pill
            key={role.name}
            text={role.display_name || role.name}
            css={{
              backgroundColor: theme.palette.background.paperLight,
              borderColor: theme.palette.divider,
            }}
          />
        ))}
      </PopoverContent>
    </Popover>
  );
}

const fallbackRole: Role = {
  name: "member",
  display_name: "Member",
} as const;

const roleNamesByAccessLevel: readonly string[] = [
  "owner",
  "user-admin",
  "template-admin",
  "auditor",
];

function sortRolesByAccessLevel(roles: Role[]) {
  if (roles.length === 0) {
    return roles;
  }

  return [...roles].sort(
    (r1, r2) =>
      roleNamesByAccessLevel.indexOf(r1.name) -
      roleNamesByAccessLevel.indexOf(r2.name),
  );
}

function getSelectedRoleNames(roles: readonly Role[]) {
  const roleNameSet = new Set(roles.map((role) => role.name));
  if (roleNameSet.size === 0) {
    roleNameSet.add(fallbackRole.name);
  }

  return roleNameSet;
}
