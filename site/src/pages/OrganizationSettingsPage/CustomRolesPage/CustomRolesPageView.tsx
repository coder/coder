import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation } from "react-query";
import { Link as RouterLink, useNavigate } from "react-router";
import { reportPremiumFunnelEvent } from "#/api/queries/premiumFunnel";
import type { AssignableRoles, Organization, Role } from "#/api/typesGenerated";
import { PremiumBadge } from "#/components/Badge/PresetBadges";
import { Button, Button as ShadcnButton } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { AddPlusIcon } from "#/components/Icons/AddPlusIcon";
import { LightbulbIcon } from "#/components/Icons/LightbulbIcon";
import { PREMIUM_PAGE_PATH } from "#/components/Paywall/Paywall";
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
import { trackPremiumFunnelClick } from "#/modules/paywall/premiumFunnelAttribution";
import type { Permissions } from "#/modules/permissions";
import { DefaultRolesDialog } from "./DefaultRolesDialog";
import { PermissionPillsList } from "./PermissionPillsList";

/**
 * Premium star shown in the badge of the Custom Roles upsell empty state. It is
 * decorative, so the stroke uses `currentColor` and the size is controlled by
 * the caller via `className`.
 */
const PremiumStarIcon: FC<React.SVGProps<SVGSVGElement>> = (props) => {
	return (
		<svg
			viewBox="0 0 36 36"
			fill="none"
			xmlns="http://www.w3.org/2000/svg"
			aria-hidden="true"
			{...props}
		>
			<path
				d="M18.0006 26.5909L25.6956 31.3229C25.8952 31.4441 26.1263 31.5034 26.3596 31.4933C26.593 31.4832 26.8181 31.4041 27.0065 31.2661C27.195 31.1281 27.3382 30.9373 27.4183 30.7179C27.4983 30.4985 27.5115 30.2603 27.4562 30.0334L25.3637 21.2035L32.2121 15.2973C32.3867 15.1439 32.5126 14.9429 32.5742 14.7188C32.6359 14.4948 32.6305 14.2576 32.5589 14.0365C32.4872 13.8155 32.3524 13.6202 32.1711 13.4749C31.9898 13.3296 31.7699 13.2406 31.5385 13.2188L22.5512 12.4876L19.089 4.10633C19.0007 3.8901 18.8501 3.70506 18.6562 3.57481C18.4624 3.44456 18.2341 3.375 18.0006 3.375C17.767 3.375 17.5387 3.44456 17.3449 3.57481C17.151 3.70506 17.0004 3.8901 16.9121 4.10633L13.4499 12.4876L4.46258 13.2188C4.22966 13.2393 4.00793 13.3279 3.82509 13.4737C3.64225 13.6194 3.50641 13.8158 3.43454 14.0383C3.36268 14.2608 3.35797 14.4995 3.42101 14.7247C3.48405 14.9499 3.61204 15.1515 3.78899 15.3043L10.6374 21.2105L8.54493 30.0334C8.48961 30.2603 8.5028 30.4985 8.58283 30.7179C8.66287 30.9373 8.80615 31.1281 8.99458 31.2661C9.183 31.4041 9.40811 31.4832 9.64146 31.4933C9.8748 31.5034 10.1059 31.4441 10.3056 31.3229L18.0006 26.5909Z"
				stroke="currentColor"
				strokeWidth={2.403}
				strokeLinecap="round"
			/>
			<path d="M18 19.8L18 16.2" stroke="currentColor" strokeWidth={2.4} />
		</svg>
	);
};

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
					canViewPremium={permissions.viewAllLicenses}
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
					canViewPremium={permissions.viewAllLicenses}
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
	canViewPremium: boolean;
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
	canViewPremium,
	onDeleteRole,
}) => {
	const { mutate: reportFunnelClick } = useMutation(reportPremiumFunnelEvent());

	if (roles === undefined) {
		return <TableLoader />;
	}
	if (roles.length === 0) {
		if (!isCustomRolesEnabled) {
			return (
				<TableEmpty
					icon={<PremiumStarIcon className="size-9 text-highlight-sky" />}
					message="No custom roles yet"
					description="Upgrade to a premium license to create custom roles."
					cta={
						canViewPremium && (
							<Button asChild size="sm">
								<RouterLink
									to={PREMIUM_PAGE_PATH}
									onClick={() =>
										reportFunnelClick(
											trackPremiumFunnelClick("custom_roles", "small"),
										)
									}
								>
									Start free trial
								</RouterLink>
							</Button>
						)
					}
				/>
			);
		}
		return (
			<TableEmpty
				icon={<LightbulbIcon className="size-9 text-highlight-sky" />}
				message="No custom roles yet"
				description={
					canCreateOrgRole
						? "Create your first custom role"
						: "You don't have permission to create a custom role"
				}
				cta={
					canCreateOrgRole && (
						<Button asChild>
							<RouterLink to="create">
								<AddPlusIcon className="text-highlight-sky" />
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
