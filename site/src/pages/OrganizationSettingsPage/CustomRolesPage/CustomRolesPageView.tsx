import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { AssignableRoles, Organization, Role } from "#/api/typesGenerated";
import { PremiumBadge } from "#/components/Badges/Badges";
import { Button, Button as ShadcnButton } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import { docs } from "#/utils/docs";
import { DefaultRolesDialog } from "./DefaultRolesDialog";
import { PermissionPillsList } from "./PermissionPillsList";

interface CustomRolesPageViewProps {
	organization: Organization;
	builtInRoles: AssignableRoles[] | undefined;
	customRoles: AssignableRoles[] | undefined;
	onDeleteRole: (role: Role) => void;
	canCreateOrgRole: boolean;
	canUpdateOrgRole: boolean;
	canDeleteOrgRole: boolean;
	canEditDefaultRoles: boolean;
	isCustomRolesEnabled: boolean;
	defaultRolesEnabled?: boolean;
	defaultRolesEntitled?: boolean;
	availableOrgRoles?: AssignableRoles[];
	onUpdateDefaultRoles?: (roles: string[]) => Promise<void>;
	isUpdatingDefaultRoles?: boolean;
}

export const CustomRolesPageView: FC<CustomRolesPageViewProps> = ({
	organization,
	builtInRoles,
	customRoles,
	onDeleteRole,
	canCreateOrgRole,
	canUpdateOrgRole,
	canDeleteOrgRole,
	canEditDefaultRoles,
	isCustomRolesEnabled,
	defaultRolesEnabled,
	defaultRolesEntitled,
	availableOrgRoles,
	onUpdateDefaultRoles,
	isUpdatingDefaultRoles,
}) => {
	const showDefaultRoles =
		defaultRolesEnabled && canEditDefaultRoles && Boolean(onUpdateDefaultRoles);

	return (
		<div className="flex flex-col gap-8">
			{!isCustomRolesEnabled && (
				<PaywallPremium
					message="Custom Roles"
					description="Create custom roles to grant users a tailored set of granular permissions."
					documentationLink={docs("/admin/users/groups-roles")}
				/>
			)}
			{showDefaultRoles && onUpdateDefaultRoles && (
				<DefaultRolesSection
					organization={organization}
					availableOrgRoles={availableOrgRoles}
					defaultRolesEntitled={Boolean(defaultRolesEntitled)}
					isUpdatingDefaultRoles={Boolean(isUpdatingDefaultRoles)}
					onUpdateDefaultRoles={onUpdateDefaultRoles}
				/>
			)}
			<div className="flex flex-row gap-4 items-baseline justify-between">
				<span>
					<h2 className="mb-0 text-lg">Custom Roles</h2>
					<span className="text-sm text-content-secondary leading-relaxed">
						Create custom roles to grant users a tailored set of granular
						permissions.
					</span>
				</span>
				{canCreateOrgRole && isCustomRolesEnabled && (
					<Button variant="outline" asChild>
						<RouterLink to="create">
							<PlusIcon />
							Create custom role
						</RouterLink>
					</Button>
				)}
			</div>
			<RoleTable
				roles={customRoles}
				isCustomRolesEnabled={isCustomRolesEnabled}
				canCreateOrgRole={canCreateOrgRole}
				canUpdateOrgRole={canUpdateOrgRole}
				canDeleteOrgRole={canDeleteOrgRole}
				onDeleteRole={onDeleteRole}
			/>
			<span>
				<h2 className="mb-0 text-lg">Built-In Roles</h2>
				<span className="text-sm text-content-secondary leading-relaxed">
					Built-in roles have predefined permissions. You cannot edit or delete
					built-in roles.
				</span>
			</span>
			<RoleTable
				roles={builtInRoles}
				isCustomRolesEnabled={isCustomRolesEnabled}
				canCreateOrgRole={canCreateOrgRole}
				canUpdateOrgRole={canUpdateOrgRole}
				canDeleteOrgRole={canDeleteOrgRole}
				onDeleteRole={onDeleteRole}
			/>
		</div>
	);
};

interface DefaultRolesSectionProps {
	organization: Organization;
	availableOrgRoles?: AssignableRoles[];
	defaultRolesEntitled: boolean;
	isUpdatingDefaultRoles: boolean;
	onUpdateDefaultRoles: (roles: string[]) => Promise<void>;
}

const DefaultRolesSection: FC<DefaultRolesSectionProps> = ({
	organization,
	availableOrgRoles,
	defaultRolesEntitled,
	isUpdatingDefaultRoles,
	onUpdateDefaultRoles,
}) => {
	const [isEditing, setIsEditing] = useState(false);

	return (
		<div className="flex flex-col gap-3">
			<div className="flex flex-row gap-4 items-baseline justify-between">
				<span>
					<h2 className="mb-0 text-lg flex items-center gap-2">
						Default Roles
						{!defaultRolesEntitled && <PremiumBadge />}
					</h2>
					<span className="text-sm text-content-secondary leading-relaxed">
						Roles attached to every member of this organization. An empty
						selection limits new members to the floor permissions only.
					</span>
				</span>
				<Button
					type="button"
					variant="outline"
					onClick={() => setIsEditing(true)}
					disabled={isUpdatingDefaultRoles || !defaultRolesEntitled}
				>
					Edit default roles
				</Button>
			</div>
			<div className="text-sm">
				{organization.default_org_member_roles.length === 0 ? (
					<span className="text-content-secondary">
						No default roles. New members receive only the floor.
					</span>
				) : (
					<DefaultRolesSummary
						roleNames={organization.default_org_member_roles}
						availableRoles={availableOrgRoles}
					/>
				)}
			</div>
			{!defaultRolesEntitled && (
				<p className="text-xs text-content-secondary mt-0 mb-0">
					Editing organization settings requires a Premium license.
				</p>
			)}
			<DefaultRolesDialog
				open={isEditing}
				currentRoles={organization.default_org_member_roles}
				availableRoles={availableOrgRoles}
				onCancel={() => setIsEditing(false)}
				onConfirm={async (roles) => {
					await onUpdateDefaultRoles(roles);
					setIsEditing(false);
				}}
				isUpdating={isUpdatingDefaultRoles}
			/>
		</div>
	);
};

interface DefaultRolesSummaryProps {
	roleNames: readonly string[];
	availableRoles?: AssignableRoles[];
}

const DefaultRolesSummary: FC<DefaultRolesSummaryProps> = ({
	roleNames,
	availableRoles,
}) => {
	const displayNameFor = (name: string): string => {
		const role = availableRoles?.find((r) => r.name === name);
		return role?.display_name || role?.name || name;
	};

	return (
		<ul className="list-disc pl-5 m-0 flex flex-col gap-1">
			{roleNames.map((name) => (
				<li key={name}>{displayNameFor(name)}</li>
			))}
		</ul>
	);
};

interface RoleTableProps {
	roles: AssignableRoles[] | undefined;
	isCustomRolesEnabled: boolean;
	canCreateOrgRole: boolean;
	canUpdateOrgRole: boolean;
	canDeleteOrgRole: boolean;
	onDeleteRole: (role: Role) => void;
}

const RoleTable: FC<RoleTableProps> = ({
	roles,
	isCustomRolesEnabled,
	canCreateOrgRole,
	canUpdateOrgRole,
	canDeleteOrgRole,
	onDeleteRole,
}) => {
	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead className="w-2/5">Name</TableHead>
					<TableHead className="w-3/5">Permissions</TableHead>
					<TableHead className="w-auto" />
				</TableRow>
			</TableHeader>
			<TableBody>
				<RoleTableBody
					roles={roles}
					isCustomRolesEnabled={isCustomRolesEnabled}
					canCreateOrgRole={canCreateOrgRole}
					canUpdateOrgRole={canUpdateOrgRole}
					canDeleteOrgRole={canDeleteOrgRole}
					onDeleteRole={onDeleteRole}
				/>
			</TableBody>
		</Table>
	);
};

const RoleTableBody: FC<RoleTableProps> = ({
	roles,
	isCustomRolesEnabled,
	canCreateOrgRole,
	canUpdateOrgRole,
	canDeleteOrgRole,
	onDeleteRole,
}) => {
	if (roles === undefined) {
		return <TableLoader />;
	}
	if (roles.length === 0) {
		return (
			<TableRow className="h-14">
				<TableCell colSpan={999}>
					<EmptyState
						message="No custom roles yet"
						description={
							canCreateOrgRole && isCustomRolesEnabled
								? "Create your first custom role"
								: !isCustomRolesEnabled
									? "Upgrade to a premium license to create a custom role"
									: "You don't have permission to create a custom role"
						}
						cta={
							canCreateOrgRole &&
							isCustomRolesEnabled && (
								<Button asChild>
									<RouterLink to="create">
										<PlusIcon />
										Create custom role
									</RouterLink>
								</Button>
							)
						}
					/>
				</TableCell>
			</TableRow>
		);
	}
	return (
		<>
			{[...roles]
				.sort((a, b) => a.name.localeCompare(b.name))
				.map((role) => (
					<RoleRow
						key={role.name}
						role={role}
						canUpdateOrgRole={canUpdateOrgRole}
						canDeleteOrgRole={canDeleteOrgRole}
						onDelete={() => onDeleteRole(role)}
					/>
				))}
		</>
	);
};

interface RoleRowProps {
	role: AssignableRoles;
	canUpdateOrgRole: boolean;
	canDeleteOrgRole: boolean;
	onDelete: () => void;
}

const RoleRow: FC<RoleRowProps> = ({
	role,
	onDelete,
	canUpdateOrgRole,
	canDeleteOrgRole,
}) => {
	const navigate = useNavigate();

	return (
		<TableRow data-testid={`role-${role.name}`} className="h-14">
			<TableCell>{role.display_name || role.name}</TableCell>

			<TableCell>
				<PermissionPillsList permissions={role.organization_permissions} />
			</TableCell>

			<TableCell>
				{!role.built_in && (canUpdateOrgRole || canDeleteOrgRole) && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<ShadcnButton
								size="icon-lg"
								variant="subtle"
								aria-label="Open menu"
							>
								<EllipsisVerticalIcon aria-hidden="true" />
								<span className="sr-only">Open menu</span>
							</ShadcnButton>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							{canUpdateOrgRole && (
								<DropdownMenuItem onClick={() => navigate(role.name)}>
									Edit
								</DropdownMenuItem>
							)}
							{canDeleteOrgRole && (
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onClick={onDelete}
								>
									Delete&hellip;
								</DropdownMenuItem>
							)}
						</DropdownMenuContent>
					</DropdownMenu>
				)}
			</TableCell>
		</TableRow>
	);
};

const TableLoader = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};
