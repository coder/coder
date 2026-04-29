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
import type { SlimRole } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { TableCell } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	combineGlobalAndOrgRoles,
	memberRole,
	type ScopedSlimRole,
	sortRoles,
} from "#/modules/roles";

type UserRoleCellProps = {
	globalRoles?: readonly SlimRole[];
	roles: readonly SlimRole[];
};

export const UserRoleCell: React.FC<UserRoleCellProps> = ({
	globalRoles = [],
	roles,
}) => {
	const mergedRoles = combineGlobalAndOrgRoles(globalRoles, roles);
	const [mainDisplayRole = memberRole, ...extraRoles] = sortRoles(mergedRoles);

	return (
		<TableCell>
			<div className="flex flex-row gap-1 items-center">
				<RoleBadge role={mainDisplayRole} />

				{extraRoles.length > 0 && <MoreRolePill roles={extraRoles} />}
			</div>
		</TableCell>
	);
};

type MoreRolePillProps = {
	roles: readonly ScopedSlimRole[];
};

const MoreRolePill: React.FC<MoreRolePillProps> = ({ roles }) => {
	return (
		<TooltipProvider>
			<Tooltip delayDuration={0}>
				<TooltipTrigger asChild>
					<Badge>+{roles.length} more</Badge>
				</TooltipTrigger>

				<TooltipContent className="flex flex-row flex-wrap content-around gap-x-2 gap-y-3 px-4 py-3 border-surface-quaternary">
					{roles.map((role) => (
						<RoleBadge key={role.name} role={role} />
					))}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

type RoleBadgeProps = {
	role: ScopedSlimRole;
};

const RoleBadge: React.FC<RoleBadgeProps> = ({ role }) => {
	const displayName = role.display_name || role.name;
	const isOwnerRole =
		role.name === "owner" || role.name === "organization-admin";

	return (
		<Badge
			key={role.name}
			variant={role.global ? "green" : isOwnerRole ? "purple" : "default"}
		>
			{role.global ? (
				<Tooltip>
					<TooltipTrigger asChild>
						<span>{displayName}*</span>
					</TooltipTrigger>
					<TooltipContent side="bottom" sideOffset={8}>
						This user has this role for all organizations.
					</TooltipContent>
				</Tooltip>
			) : (
				displayName
			)}
		</Badge>
	);
};
