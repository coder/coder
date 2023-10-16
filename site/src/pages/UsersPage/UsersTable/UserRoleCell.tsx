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

  // Unless the user happens to be an owner, it is physically impossible for
  // React to know how many pills should be omitted for space reasons each time
  // a new set of roles comes in. Have to do a smoke-and-mirrors routine to help
  // mask that and avoid UI flickering
  const cellRef = useRef<HTMLDivElement>(null);
  const pillContainerRef = useRef<HTMLDivElement>(null);
  const overflowButtonRef = useRef<HTMLButtonElement>(null);

  /**
   * @todo – The logic only works properly on the first render - the
   * moment you update a user's permissions, the UI doesn't do anything, even
   * with the gnarly manual state sync in place. The cached update is
   * triggering, but not the
   *
   * Likely causes:
   * 1. Mutation logic isn't getting applied properly
   * 2. Trace through the parent component logic and see if I need to update
   *    things to reference the roles prop instead of user.roles
   */

  const roleDisplayInfo = getRoleDisplayInfo(user.roles);

  // Have to do manual state syncs to make sure that cells change as roles get
  // updated; there isn't a good render key to use to simplify this, and the
  // MutationObserver API doesn't work with React's order of operations well
  // enough to avoid flickering - it's just not fast enough in the right ways
  const [cachedUser, setCachedUser] = useState(user);
  const [rolesToTruncate, setRolesToTruncate] = useState(
    roleDisplayInfo.hasOwner
      ? user.roles.length - roleDisplayInfo.roles.length
      : null,
  );

  if (user !== cachedUser) {
    const needTruncationSync =
      !roleDisplayInfo.hasOwner &&
      (user.roles.length !== cachedUser.roles.length ||
        user.roles.every((role, index) => role === cachedUser.roles[index]));

    setCachedUser(user);
    console.log("huh");

    // This isn't ever triggering, even if you update the permissions for a
    // user
    if (needTruncationSync) {
      setRolesToTruncate(null);
    }
  }

  // Mutates the contents of the pill container to hide overflowing content on
  // the first render, and then updates rolesToTruncate so that these overflow
  // calculations can be done with 100% pure state/props calculations for all
  // re-renders (at least until the roles list changes by content again)
  useLayoutEffect(() => {
    const cell = cellRef.current;
    const pillContainer = pillContainerRef.current;
    if (rolesToTruncate !== null || cell === null || pillContainer === null) {
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
  }, [rolesToTruncate]);

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

              return (
                <Pill
                  key={role.name}
                  text={role.display_name}
                  css={{
                    backgroundColor: isOwnerRole
                      ? theme.palette.info.dark
                      : theme.palette.background.paperLight,
                    borderColor: isOwnerRole
                      ? theme.palette.info.light
                      : theme.palette.divider,
                  }}
                />
              );
            })}
          </Stack>

          {/*
           * Have to render this, even when rolesToTruncate is null, in order
           * for the layoutEffect trick to work properly
           */}
          {rolesToTruncate !== 0 && (
            <Pill
              text={getOverflowButtonText(rolesToTruncate ?? 0)}
              css={{
                backgroundColor: theme.palette.background.paperLight,
                borderColor: theme.palette.divider,
              }}
            />
          )}
        </Stack>
      </Stack>
    </TableCell>
  );
}
