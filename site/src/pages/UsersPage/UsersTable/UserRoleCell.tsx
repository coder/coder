import { makeStyles } from "@mui/styles";
import { type User, type Role } from "api/typesGenerated";
import { combineClasses } from "utils/combineClasses";

import { EditRolesButton } from "./EditRolesButton";
import { Pill } from "components/Pill/Pill";
import TableCell from "@mui/material/TableCell";
import Stack from "@mui/material/Stack";

const useStyles = makeStyles((theme) => ({
  rolePill: {
    backgroundColor: theme.palette.background.paperLight,
    borderColor: theme.palette.divider,
  },
  rolePillOwner: {
    backgroundColor: theme.palette.info.dark,
    borderColor: theme.palette.info.light,
  },
}));

const roleOrder = ["owner", "user-admin", "template-admin", "auditor"];

const sortRoles = (roles: readonly Role[]) => {
  return [...roles].sort(
    (a, b) => roleOrder.indexOf(a.name) - roleOrder.indexOf(b.name),
  );
};

type Props = {
  canEditUsers: boolean;
  roles: undefined | readonly Role[];
  user: User;
  isLoading: boolean;
  oidcRoleSyncEnabled: boolean;
  onUserRolesUpdate: (user: User, newRoleNames: string[]) => void;
};

// When the user has no role we want to show they are a Member
const fallbackRole: Role = {
  name: "member",
  display_name: "Member",
} as const;

export function UserRoleCell({
  canEditUsers,
  roles,
  user,
  isLoading,
  oidcRoleSyncEnabled,
  onUserRolesUpdate,
}: Props) {
  const styles = useStyles();

  const userRoles =
    user.roles.length === 0 ? [fallbackRole] : sortRoles(user.roles);

  return (
    <TableCell>
      <Stack direction="row" spacing={1}>
        {canEditUsers && (
          <EditRolesButton
            roles={roles ? sortRoles(roles) : []}
            selectedRoles={userRoles}
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

        {userRoles.map((role) => (
          <Pill
            key={role.name}
            text={role.display_name}
            className={combineClasses({
              [styles.rolePill]: true,
              [styles.rolePillOwner]: role.name === "owner",
            })}
          />
        ))}
      </Stack>
    </TableCell>
  );
}
