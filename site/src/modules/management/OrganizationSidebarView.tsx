import { cx } from "@emotion/css";
import AddIcon from "@mui/icons-material/Add";
import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem as SidebarNavSubItem,
} from "components/Sidebar/Sidebar";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import type { Permissions } from "contexts/auth/permissions";
import { type ClassName, useClassName } from "hooks/useClassName";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC, ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";

export interface OrganizationWithPermissions extends Organization {
	permissions: AuthorizationResponse;
}

interface SidebarProps {
	/** The active org name, if any.  Overrides activeSettings. */
	activeOrganizationName: string | undefined;
	/** Organizations and their permissions or undefined if still fetching. */
	organizations: OrganizationWithPermissions[] | undefined;
	/** Site-wide permissions. */
	permissions: Permissions;
}

/**
 * Organization settings left sidebar menu.
 */
export const OrganizationSidebarView: FC<SidebarProps> = ({
	activeOrganizationName,
	organizations,
	permissions,
}) => {
	const { showOrganizations } = useDashboard();

	return (
		<BaseSidebar>
			{showOrganizations && (
				<OrganizationsSettingsNavigation
					activeOrganizationName={activeOrganizationName}
					organizations={organizations}
					permissions={permissions}
				/>
			)}
		</BaseSidebar>
	);
};

function urlForSubpage(organizationName: string, subpage = ""): string {
	return `/organizations/${organizationName}/${subpage}`;
}

interface OrganizationsSettingsNavigationProps {
	/** The active org name if an org is being viewed. */
	activeOrganizationName: string | undefined;
	/** Organizations and their permissions or undefined if still fetching. */
	organizations: OrganizationWithPermissions[] | undefined;
	/** Site-wide permissions. */
	permissions: Permissions;
}

/**
 * Displays navigation for all organizations and a create organization link.
 *
 * If organizations or their permissions are still loading, show a loader.
 *
 * If there are no organizations and the user does not have the create org
 * permission, nothing is displayed.
 */
const OrganizationsSettingsNavigation: FC<
	OrganizationsSettingsNavigationProps
> = ({ activeOrganizationName, organizations, permissions }) => {
	// Wait for organizations and their permissions to load in.
	if (!organizations) {
		return <Loader />;
	}

	if (organizations.length <= 0 && !permissions.createOrganization) {
		return null;
	}

	return (
		<>
			{permissions.createOrganization && (
				<SidebarNavItem
					active="auto"
					href="/organizations/new"
					icon={<AddIcon />}
				>
					New organization
				</SidebarNavItem>
			)}
			{organizations.map((org) => (
				<OrganizationSettingsNavigation
					key={org.id}
					organization={org}
					active={org.name === activeOrganizationName}
				/>
			))}
		</>
	);
};

interface OrganizationSettingsNavigationProps {
	/** Whether this organization is currently selected. */
	active: boolean;
	/** The organization to display in the navigation. */
	organization: OrganizationWithPermissions;
}

/**
 * Displays navigation for a single organization.
 *
 * If inactive, no sub-menu items will be shown, just the organization name.
 *
 * If active, it will show sub-menu items based on the permissions.
 */
const OrganizationSettingsNavigation: FC<
	OrganizationSettingsNavigationProps
> = ({ active, organization }) => {
	return (
		<>
			<SidebarNavItem
				active={active}
				href={urlForSubpage(organization.name)}
				icon={
					<UserAvatar
						key={organization.id}
						size="sm"
						username={organization.display_name}
						avatarURL={organization.icon}
					/>
				}
			>
				{organization.display_name}
			</SidebarNavItem>
			{active && (
				<div className="flex flex-col gap-1 my-2 ml-11">
					{organization.permissions.editOrganization && (
						<SidebarNavSubItem end href={urlForSubpage(organization.name)}>
							Settings
						</SidebarNavSubItem>
					)}
					{organization.permissions.editMembers && (
						<SidebarNavSubItem
							href={urlForSubpage(organization.name, "members")}
						>
							Members
						</SidebarNavSubItem>
					)}
					{organization.permissions.editGroups && (
						<SidebarNavSubItem
							href={urlForSubpage(organization.name, "groups")}
						>
							Groups
						</SidebarNavSubItem>
					)}
					{organization.permissions.assignOrgRole && (
						<SidebarNavSubItem href={urlForSubpage(organization.name, "roles")}>
							Roles
						</SidebarNavSubItem>
					)}
					{organization.permissions.viewProvisioners && (
						<SidebarNavSubItem
							href={urlForSubpage(organization.name, "provisioners")}
						>
							Provisioners
						</SidebarNavSubItem>
					)}
					{organization.permissions.viewIdpSyncSettings && (
						<SidebarNavSubItem
							href={urlForSubpage(organization.name, "idp-sync")}
						>
							IdP Sync
						</SidebarNavSubItem>
					)}
				</div>
			)}
		</>
	);
};

interface SidebarNavItemProps {
	active?: boolean | "auto";
	children?: ReactNode;
	icon?: ReactNode;
	href: string;
}

const SidebarNavItem: FC<SidebarNavItemProps> = ({
	active,
	children,
	href,
	icon,
}) => {
	const link = useClassName(classNames.link, []);
	const activeLink = useClassName(classNames.activeLink, []);

	const content = (
		<Stack alignItems="center" spacing={1.5} direction="row">
			{icon}
			{children}
		</Stack>
	);

	if (active === "auto") {
		return (
			<NavLink
				to={href}
				className={({ isActive }) => cx([link, isActive && activeLink])}
			>
				{content}
			</NavLink>
		);
	}

	return (
		<Link to={href} className={cx([link, active && activeLink])}>
			{content}
		</Link>
	);
};

const classNames = {
	link: (css, theme) => css`
    color: inherit;
    display: block;
    font-size: 14px;
    text-decoration: none;
    padding: 10px 12px 10px 16px;
    border-radius: 4px;
    transition: background-color 0.15s ease-in-out;
    position: relative;

    &:hover {
      background-color: ${theme.palette.action.hover};
    }

    border-left: 3px solid transparent;
  `,

	activeLink: (css, theme) => css`
    border-left-color: ${theme.palette.primary.main};
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
  `,
} satisfies Record<string, ClassName>;
