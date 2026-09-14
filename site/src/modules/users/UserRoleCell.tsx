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
