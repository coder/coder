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
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Tooltip from "@mui/material/Tooltip";
import type { LoginType, SlimRole } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import { TableCell } from "components/Table/Table";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import type { FC } from "react";
import { EditRolesButton } from "./EditRolesButton";

type UserRoleCellProps = {
	isLoading: boolean;
	canEditUsers: boolean;
	allAvailableRoles: readonly SlimRole[] | undefined;
	userLoginType?: LoginType;
	inheritedRoles?: readonly SlimRole[];
	roles: readonly SlimRole[];
	oidcRoleSyncEnabled: boolean;
	onEditRoles: (newRoleNames: string[]) => void;
};

export const UserRoleCell: FC<UserRoleCellProps> = ({
	isLoading,
	canEditUsers,
	allAvailableRoles,
	userLoginType,
	inheritedRoles,
	roles,
	oidcRoleSyncEnabled,
	onEditRoles,
}) => {
	const mergedRoles = getTieredRoles(inheritedRoles ?? [], roles);
	const [mainDisplayRole = fallbackRole, ...extraRoles] =
		sortRolesByAccessLevel(mergedRoles ?? []);
	const hasOwnerRole =
		mainDisplayRole.name === "owner" ||
		mainDisplayRole.name === "organization-admin";

	const displayName = mainDisplayRole.display_name || mainDisplayRole.name;

	return (
		<TableCell>
			<div className="flex flex-row gap-1 items-center">
				{canEditUsers && (
					<EditRolesButton
						roles={sortRolesByAccessLevel(allAvailableRoles ?? [])}
						selectedRoleNames={getSelectedRoleNames(roles)}
						isLoading={isLoading}
						userLoginType={userLoginType}
						oidcRoleSync={oidcRoleSyncEnabled}
						onChange={(roles) => {
							// Remove the fallback role because it is only for the UI
							const rolesWithoutFallback = roles.filter(
								(role) => role !== fallbackRole.name,
							);

							onEditRoles(rolesWithoutFallback);
						}}
					/>
				)}

				<Pill
					css={
						hasOwnerRole
							? styles.ownerRoleBadge
							: mainDisplayRole.global
								? styles.globalRoleBadge
								: styles.roleBadge
					}
				>
					{mainDisplayRole.global ? (
						<Tooltip title="This user has this role for all organizations.">
							<span>{displayName}*</span>
						</Tooltip>
					) : (
						displayName
					)}
				</Pill>

				{extraRoles.length > 0 && <OverflowRolePill roles={extraRoles} />}
			</div>
		</TableCell>
	);
};

type OverflowRolePillProps = {
	roles: readonly TieredSlimRole[];
};

const OverflowRolePill: FC<OverflowRolePillProps> = ({ roles }) => {
	const theme = useTheme();

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<Pill
					css={{
						backgroundColor: theme.palette.background.paper,
						borderColor: theme.palette.divider,
					}}
				>
					+{roles.length} more
				</Pill>
			</PopoverTrigger>

			<PopoverContent
				disableRestoreFocus
				disableScrollLock
				css={{
					".MuiPaper-root": {
						display: "flex",
						flexFlow: "row wrap",
						columnGap: 8,
						rowGap: 12,
						padding: "12px 16px",
						alignContent: "space-around",
						minWidth: "auto",
					},
				}}
				anchorOrigin={{ vertical: -4, horizontal: "center" }}
				transformOrigin={{ vertical: "bottom", horizontal: "center" }}
			>
				{roles.map((role) => (
					<Pill
						key={role.name}
						css={role.global ? styles.globalRoleBadge : styles.roleBadge}
					>
						{role.global ? (
							<Tooltip title="This user has this role for all organizations.">
								<span>{role.display_name || role.name}*</span>
							</Tooltip>
						) : (
							role.display_name || role.name
						)}
					</Pill>
				))}
			</PopoverContent>
		</Popover>
	);
};

const styles = {
	globalRoleBadge: (theme) => ({
		backgroundColor: theme.roles.active.background,
		borderColor: theme.roles.active.outline,
	}),
	ownerRoleBadge: (theme) => ({
		backgroundColor: theme.roles.notice.background,
		borderColor: theme.roles.notice.outline,
	}),
	roleBadge: (theme) => ({
		backgroundColor: theme.experimental.l2.background,
		borderColor: theme.experimental.l2.outline,
	}),
} satisfies Record<string, Interpolation<Theme>>;

const fallbackRole: TieredSlimRole = {
	name: "member",
	display_name: "Member",
} as const;

const roleNamesByAccessLevel: readonly string[] = [
	"owner",
	"organization-admin",
	"user-admin",
	"organization-user-admin",
	"template-admin",
	"organization-template-admin",
	"auditor",
	"organization-auditor",
];

function sortRolesByAccessLevel<T extends SlimRole>(
	roles: readonly T[],
): readonly T[] {
	if (roles.length === 0) {
		return roles;
	}

	return [...roles].sort(
		(r1, r2) =>
			roleNamesByAccessLevel.indexOf(r1.name) -
			roleNamesByAccessLevel.indexOf(r2.name),
	);
}

function getSelectedRoleNames(roles: readonly SlimRole[]) {
	const roleNameSet = new Set(roles.map((role) => role.name));
	if (roleNameSet.size === 0) {
		roleNameSet.add(fallbackRole.name);
	}

	return roleNameSet;
}

interface TieredSlimRole extends SlimRole {
	global?: boolean;
}

function getTieredRoles(
	globalRoles: readonly SlimRole[],
	localRoles: readonly SlimRole[],
) {
	const roles = new Map<string, TieredSlimRole>();

	for (const role of globalRoles) {
		roles.set(role.name, {
			...role,
			global: true,
		});
	}
	for (const role of localRoles) {
		if (roles.has(role.name)) {
			continue;
		}
		roles.set(role.name, role);
	}

	return [...roles.values()];
}
