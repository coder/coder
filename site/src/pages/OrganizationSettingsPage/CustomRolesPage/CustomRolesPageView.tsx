import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { AssignableRoles, Organization, Role } from "#/api/typesGenerated";
import { PremiumBadge } from "#/components/Badge/PresetBadges";
import { Button, Button as ShadcnButton } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import { PremiumPaywallSmall } from "#/modules/paywall/PremiumPaywallSmall";
import type { Permissions } from "#/modules/permissions";
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
	permissions: Permissions;
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
	permissions,
	defaultRolesEntitled,
	availableOrgRoles,
	onUpdateDefaultRoles,
	isUpdatingDefaultRoles,
}) => {
	return (
		<div className="flex flex-col gap-12">
			{!isCustomRolesEnabled && (
				<PremiumPaywallSmall
					source="custom_roles"
					message="Custom Roles"
					description="Build roles with the exact permissions your team needs."
					features={[
						"Configure roles per organization",
						"Go beyond the built-in role set",
						"Assign custom roles to any user",
					]}
					canViewPremium={permissions.viewAllLicenses}
				/>
			)}
			{onUpdateDefaultRoles && (
				<DefaultRolesSection
					organization={organization}
					availableOrgRoles={availableOrgRoles}
					canEditDefaultRoles={canEditDefaultRoles}
					defaultRolesEntitled={Boolean(defaultRolesEntitled)}
					isUpdatingDefaultRoles={Boolean(isUpdatingDefaultRoles)}
					onUpdateDefaultRoles={onUpdateDefaultRoles}
				/>
			)}
			<div>
				<SettingsHeader
					actions={
						canCreateOrgRole &&
						isCustomRolesEnabled && (
							<Button variant="outline" asChild>
								<RouterLink to="create">
									<PlusIcon />
									Create custom role
								</RouterLink>
							</Button>
						)
					}
				>
					<SettingsHeaderTitle level="h2" hierarchy="secondary">
						Custom Roles
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Create custom roles to grant users a tailored set of granular
						permissions.
					</SettingsHeaderDescription>
				</SettingsHeader>
				<RoleTable
					roles={customRoles}
					isCustomRolesEnabled={isCustomRolesEnabled}
					canCreateOrgRole={canCreateOrgRole}
					canUpdateOrgRole={canUpdateOrgRole}
					canDeleteOrgRole={canDeleteOrgRole}
					onDeleteRole={onDeleteRole}
					aria-label="Custom roles"
				/>
			</div>
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle level="h2" hierarchy="secondary">
						Built-In Roles
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Built-in roles have predefined permissions. You cannot edit or
						delete built-in roles.
					</SettingsHeaderDescription>
				</SettingsHeader>
				<RoleTable
					roles={builtInRoles}
					isCustomRolesEnabled={isCustomRolesEnabled}
					canCreateOrgRole={canCreateOrgRole}
					canUpdateOrgRole={canUpdateOrgRole}
					canDeleteOrgRole={canDeleteOrgRole}
					onDeleteRole={onDeleteRole}
					aria-label="Built-in roles"
				/>
			</div>
		</div>
	);
};

interface DefaultRolesSectionProps {
	organization: Organization;
	availableOrgRoles?: AssignableRoles[];
	canEditDefaultRoles: boolean;
	defaultRolesEntitled: boolean;
	isUpdatingDefaultRoles: boolean;
	onUpdateDefaultRoles: (roles: string[]) => Promise<void>;
}

const DefaultRolesSection: FC<DefaultRolesSectionProps> = ({
	organization,
	availableOrgRoles,
	canEditDefaultRoles,
	defaultRolesEntitled,
	isUpdatingDefaultRoles,
	onUpdateDefaultRoles,
}) => {
	const [isEditing, setIsEditing] = useState(false);

	return (
		<div>
			<SettingsHeader
				actions={
					canEditDefaultRoles && (
						<Button
							type="button"
							variant="outline"
							onClick={() => setIsEditing(true)}
							disabled={
								isUpdatingDefaultRoles ||
								!defaultRolesEntitled ||
								!availableOrgRoles
							}
						>
							Edit default roles
						</Button>
					)
				}
			>
				<SettingsHeaderTitle level="h2" hierarchy="secondary">
					Default Roles
					{!defaultRolesEntitled && <PremiumBadge />}
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Roles granted to every member of this organization, current and
					future, in addition to any roles assigned directly. Removing a role
					here removes it from all members that are not assigned that role
					directly.
					{!defaultRolesEntitled && (
						<> Editing organization settings requires a Premium license.</>
					)}
				</SettingsHeaderDescription>
			</SettingsHeader>
			<div className="text-sm">
				{organization.default_org_member_roles.length === 0 ? (
					<span className="text-content-secondary">
						No default roles. Members have only the permissions of their
						directly assigned roles, which excludes creating and using
						workspaces.
					</span>
				) : (
					<DefaultRolesSummary
						roleNames={organization.default_org_member_roles}
						availableRoles={availableOrgRoles}
					/>
				)}
			</div>
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

interface RoleTableBodyProps {
	roles: AssignableRoles[] | undefined;
	isCustomRolesEnabled: boolean;
	canCreateOrgRole: boolean;
	canUpdateOrgRole: boolean;
	canDeleteOrgRole: boolean;
	onDeleteRole: (role: Role) => void;
}

interface RoleTableProps extends RoleTableBodyProps {
	"aria-label": string;
}

const RoleTable: FC<RoleTableProps> = ({
	"aria-label": ariaLabel,
	...bodyProps
}) => {
	return (
		<Table aria-label={ariaLabel}>
			<TableHeader>
				<TableRow>
					<TableHead className="w-2/5">Name</TableHead>
					<TableHead className="w-3/5">Permissions</TableHead>
					<TableHead className="w-auto" />
				</TableRow>
			</TableHeader>
			<TableBody>
				<RoleTableBody {...bodyProps} />
			</TableBody>
		</Table>
	);
};

const RoleTableBody: FC<RoleTableBodyProps> = ({
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
			<TableEmpty
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
