import { cx } from "@emotion/css";
import type { Interpolation, Theme } from "@emotion/react";
import AddIcon from "@mui/icons-material/Add";
import SettingsIcon from "@mui/icons-material/Settings";
import type {
	AuthorizationResponse,
	Experiments,
	Organization,
} from "api/typesGenerated";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Loader } from "components/Loader/Loader";
import { Sidebar as BaseSidebar } from "components/Sidebar/Sidebar";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { type ClassName, useClassName } from "hooks/useClassName";
import { useDashboard } from "modules/dashboard/useDashboard";
import { linkToUsers } from "modules/navigation";
import type { FC, ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";

export interface OrganizationWithPermissions extends Organization {
	permissions: AuthorizationResponse;
}

interface SidebarProps {
	/** True if a settings page is being viewed. */
	activeSettings: boolean;
	/** The active org name, if any.  Overrides activeSettings. */
	activeOrganizationName: string | undefined;
	/** Organizations and their permissions or undefined if still fetching. */
	organizations: OrganizationWithPermissions[] | undefined;
	/** Site-wide permissions. */
	permissions: AuthorizationResponse;
}

/**
 * A combined deployment settings and organization menu.
 */
export const SidebarView: FC<SidebarProps> = ({
	activeSettings,
	activeOrganizationName,
	organizations,
	permissions,
}) => {
	const { showOrganizations } = useDashboard();

	// TODO: Do something nice to scroll to the active org.
	return (
		<BaseSidebar>
			{showOrganizations && (
				<header>
					<h2 css={styles.sidebarHeader}>Deployment</h2>
				</header>
			)}

			<DeploymentSettingsNavigation
				active={!activeOrganizationName && activeSettings}
				permissions={permissions}
			/>
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

interface DeploymentSettingsNavigationProps {
	/** Whether a deployment setting page is being viewed. */
	active: boolean;
	/** Site-wide permissions. */
	permissions: AuthorizationResponse;
}

/**
 * Displays navigation for deployment settings.  If active, highlight the main
 * menu heading.
 *
 * Menu items are shown based on the permissions.  If organizations can be
 * viewed, groups are skipped since they will show under each org instead.
 */
const DeploymentSettingsNavigation: FC<DeploymentSettingsNavigationProps> = ({
	active,
	permissions,
}) => {
	return (
		<div css={{ paddingBottom: 12 }}>
			<SidebarNavItem
				active={active}
				href={
					permissions.viewDeploymentValues
						? "/deployment/general"
						: "/deployment/workspace-proxies"
				}
				// 24px matches the width of the organization icons, and the component
				// is smart enough to keep the icon itself square. It looks too big if
				// it's 24x24.
				icon={<SettingsIcon css={{ width: 24, height: 20 }} />}
			>
				Deployment
			</SidebarNavItem>
			{active && (
				<Stack spacing={0.5} css={{ marginBottom: 8, marginTop: 8 }}>
					{permissions.viewDeploymentValues && (
						<SidebarNavSubItem href="general">General</SidebarNavSubItem>
					)}
					{permissions.viewAllLicenses && (
						<SidebarNavSubItem href="licenses">Licenses</SidebarNavSubItem>
					)}
					{permissions.editDeploymentValues && (
						<SidebarNavSubItem href="appearance">Appearance</SidebarNavSubItem>
					)}
					{permissions.viewDeploymentValues && (
						<SidebarNavSubItem href="userauth">
							User Authentication
						</SidebarNavSubItem>
					)}
					{permissions.viewDeploymentValues && (
						<SidebarNavSubItem href="external-auth">
							External Authentication
						</SidebarNavSubItem>
					)}
					{/* Not exposing this yet since token exchange is not finished yet.
          <SidebarNavSubItem href="oauth2-provider/ap>
            OAuth2 Applications
          </SidebarNavSubItem>*/}
					{permissions.viewDeploymentValues && (
						<SidebarNavSubItem href="network">Network</SidebarNavSubItem>
					)}
					{/* All users can view workspace regions.  */}
					<SidebarNavSubItem href="workspace-proxies">
						Workspace Proxies
					</SidebarNavSubItem>
					{permissions.viewDeploymentValues && (
						<SidebarNavSubItem href="security">Security</SidebarNavSubItem>
					)}
					{permissions.viewDeploymentValues && (
						<SidebarNavSubItem href="observability">
							Observability
						</SidebarNavSubItem>
					)}
					{permissions.viewAllUsers && (
						<SidebarNavSubItem href="users">Users</SidebarNavSubItem>
					)}
					<SidebarNavSubItem href="notifications">
						<Stack direction="row" alignItems="center" spacing={1}>
							<span>Notifications</span>
							<FeatureStageBadge contentType="beta" size="sm" />
						</Stack>
					</SidebarNavSubItem>
				</Stack>
			)}
		</div>
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
	permissions: AuthorizationResponse;
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
			<header
				css={{
					display: "flex",
					flexFlow: "row wrap",
					columnGap: "8px",
					alignItems: "baseline",
				}}
			>
				<h2 css={styles.sidebarHeader}>Organizations</h2>
				<FeatureStageBadge contentType="beta" size="sm" />
			</header>

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
	const { experiments } = useDashboard();

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
				<Stack spacing={0.5} css={{ marginBottom: 8, marginTop: 8 }}>
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
				</Stack>
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

interface SidebarNavSubItemProps {
	children?: ReactNode;
	href: string;
	end?: boolean;
}

const SidebarNavSubItem: FC<SidebarNavSubItemProps> = ({
	children,
	href,
	end,
}) => {
	const link = useClassName(classNames.subLink, []);
	const activeLink = useClassName(classNames.activeSubLink, []);

	return (
		<NavLink
			end={end}
			to={href}
			className={({ isActive }) => cx([link, isActive && activeLink])}
		>
			{children}
		</NavLink>
	);
};

const styles = {
	sidebarHeader: {
		textTransform: "uppercase",
		letterSpacing: "0.1em",
		margin: 0,
		fontSize: 11,
		fontWeight: 500,
		paddingBottom: 4,
	},
} satisfies Record<string, Interpolation<Theme>>;

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

	subLink: (css, theme) => css`
    color: ${theme.palette.text.secondary};
    text-decoration: none;

    display: block;
    font-size: 13px;
    margin-left: 44px;
    padding: 4px 12px;
    border-radius: 4px;
    transition: background-color 0.15s ease-in-out;
    margin-bottom: 1px;
    position: relative;

    &:hover {
	  color: ${theme.palette.text.primary};
      background-color: ${theme.palette.action.hover};
    }
  `,

	activeSubLink: (css, theme) => css`
	color: ${theme.palette.text.primary};
    font-weight: 600;
  `,
} satisfies Record<string, ClassName>;
