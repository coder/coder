import { useLayoutEffect, useRef, useState } from "react";
import { useTheme } from "@emotion/react";
import { type User, type Role } from "api/typesGenerated";

import { EditRolesButton } from "./EditRolesButton";
import { Pill } from "components/Pill/Pill";
import TableCell from "@mui/material/TableCell";
import Stack from "@mui/material/Stack";

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

function sortRolesByAccessLevel(roles: readonly Role[]) {
  return [...roles].sort(
    (r1, r2) =>
      roleNamesByAccessLevel.indexOf(r1.name) -
      roleNamesByAccessLevel.indexOf(r2.name),
  );
}

type RoleDisplayInfo = Readonly<{
  hasOwner: boolean;
  roles: readonly Role[];
}>;

function getRoleDisplayInfo(userRoles: readonly Role[]): RoleDisplayInfo {
  if (userRoles.length === 0) {
    return {
      hasOwner: false,
      roles: [fallbackRole],
    };
  }

  const matchedOwnerRole = userRoles.find((role) => role.name === "owner");
  if (matchedOwnerRole !== undefined) {
    return {
      hasOwner: true,
      roles: [matchedOwnerRole],
    };
  }

  const sortedRoles = [...userRoles].sort((r1, r2) => {
    if (r1.name === r2.name) {
      return 0;
    }

    return r1.name < r2.name ? -1 : 1;
  });

  return { hasOwner: false, roles: sortedRoles };
}

function getSelectedRoleNames(roles: readonly Role[]) {
  const roleNameSet = new Set(roles.map((role) => role.name));
  if (roleNameSet.size === 0) {
    roleNameSet.add(fallbackRole.name);
  }

  return roleNameSet;
}

// Defined as a function to ensure that render approach and mutation approaches
// in the component don't get out of sync
function getOverflowButtonText(overflowCount: number) {
  return `+${overflowCount} more`;
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

  const cellRef = useRef<HTMLDivElement>(null);
  const pillContainerRef = useRef<HTMLDivElement>(null);
  const overflowButtonRef = useRef<HTMLButtonElement>(null);

  // Unless the user happens to be an owner, it is physically impossible for
  // React to know how many pills should be omitted for space reasons on the
  // first render, because that info comes from real DOM nodes, which can't
  // exist until the first render pass. Have to do a smoke-and-mirrors routine
  // to help mask that and avoid UI flickering
  const roleDisplayInfo = getRoleDisplayInfo(user.roles);
  const [rolesToTruncate, setRolesToTruncate] = useState(
    roleDisplayInfo.hasOwner
      ? user.roles.length - roleDisplayInfo.roles.length
      : null,
  );

  // Mutates the contents of the pill container to hide overflowing content on
  // the first render, and then updates rolesToTruncate so that these overflow
  // calculations can be done with 100% pure state/props calculations for all
  // re-renders
  useLayoutEffect(() => {
    const cell = cellRef.current;
    const pillContainer = pillContainerRef.current;
    if (roleDisplayInfo.hasOwner || cell === null || pillContainer === null) {
      return;
    }

    let nodesRemoved = 0;
    const childrenCopy = [...pillContainer.children];

    for (let i = childrenCopy.length - 1; i >= 0; i--) {
      const child = childrenCopy[i] as HTMLElement;
      if (pillContainer.clientWidth <= cell.clientWidth) {
        break;
      }

      // Can't remove child, because then React will freak out about DOM nodes
      // disappearing in ways it wasn't aware of; have to rely on CSS styling
      child.style.visibility = "none";
      nodesRemoved++;
    }

    setRolesToTruncate(nodesRemoved);
    if (overflowButtonRef.current !== null) {
      const mutationText = getOverflowButtonText(nodesRemoved);
      overflowButtonRef.current.innerText = mutationText;
    }
  }, [roleDisplayInfo.hasOwner]);

  const finalRoleList = roleDisplayInfo.roles;

  return (
    <TableCell ref={cellRef}>
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

        <Stack direction="row" spacing={1}>
          <Stack direction="row" spacing={1} ref={pillContainerRef}>
            {finalRoleList.map((role) => {
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
                    borderColor: isOwnerRole
                      ? palette.info.light
                      : palette.divider,
                  }}
                />
              );
            })}
          </Stack>

          {rolesToTruncate !== 0 && (
            <button ref={overflowButtonRef}>
              {getOverflowButtonText(rolesToTruncate ?? 0)}
            </button>
          )}
        </Stack>
      </Stack>
    </TableCell>
  );
}
