import { cx } from "@emotion/css";
import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import {
	Command,
	CommandGroup,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import { Loader } from "components/Loader/Loader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem as SidebarNavSubItem,
} from "components/Sidebar/Sidebar";
import { Stack } from "components/Stack/Stack";
import type { Permissions } from "contexts/auth/permissions";
import { type ClassName, useClassName } from "hooks/useClassName";
import { ChevronDown, Plus } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, type ReactNode, useState } from "react";
import { Link, NavLink } from "react-router-dom";

export interface OrganizationWithPermissions extends Organization {
	permissions: AuthorizationResponse;
}

interface SidebarProps {
	/** The active org name, if any.  Overrides activeSettings. */
	activeOrganization: Organization | undefined;
	/** Organizations and their permissions or undefined if still fetching. */
	organizations: OrganizationWithPermissions[] | undefined;
	/** Site-wide permissions. */
	permissions: Permissions;
}

/**
 * Organization settings left sidebar menu.
 */
export const OrganizationSidebarView: FC<SidebarProps> = ({
	activeOrganization,
	organizations,
	permissions,
}) => {
	const { showOrganizations } = useDashboard();

	return (
		<BaseSidebar>
			{showOrganizations && (
				<OrganizationsSettingsNavigation
					activeOrganization={activeOrganization}
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
	activeOrganization: Organization | undefined;
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
> = ({ activeOrganization, organizations, permissions }) => {
	// Wait for organizations and their permissions to load in.
	if (!organizations) {
		return <Loader />;
	}

	if (organizations.length <= 0 && !permissions.createOrganization) {
		return null;
	}

	const [popoverOpen, setPopoverOpen] = useState(false);

	return (
		<>
			<Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
				<PopoverTrigger asChild>
					<Button
						variant="outline"
						aria-expanded={popoverOpen}
						className="w-60 justify-between p-2 h-10"
					>
						<div className="flex flex-row gap-2 items-center p-2">
							{activeOrganization && (
								<Avatar
									size="sm"
									src={activeOrganization.icon}
									fallback={activeOrganization.display_name}
								/>
							)}
							{activeOrganization?.display_name || activeOrganization?.name}
						</div>
						<ChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
					</Button>
				</PopoverTrigger>
				<PopoverContent align="start" className="w-60">
					<Command>
						<CommandList>
							<CommandGroup>
								{organizations.map((organization) => (
									<Link
										key={organization.id}
										to={urlForSubpage(organization.name)}
										className="no-underline visited:text-content-secondary"
									>
										<CommandItem
											value={organization.name}
											onSelect={() => {
												setPopoverOpen(false);
											}}
										>
											<Avatar
												size="sm"
												src={organization.icon}
												fallback={organization.display_name}
											/>
											{organization.display_name || organization.name}
										</CommandItem>
									</Link>
								))}
								{permissions.createOrganization && (
									<>
										<hr className="h-px border-none bg-border" />
										<Button variant="subtle" className="w-full">
											<a
												href="/organizations/new"
												className="flex items-center gap-1 no-underline hover:text-content-primary visited:text-content-secondary"
											>
												<Plus /> Create Organization
											</a>
										</Button>
									</>
								)}
							</CommandGroup>
						</CommandList>
					</Command>
				</PopoverContent>
			</Popover>
			{organizations.map((org) => (
				<OrganizationSettingsNavigation
					key={org.id}
					organization={org}
					active={org.name === activeOrganization?.name}
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
			{active && (
				<div className="flex flex-col gap-1 my-2">
					{organization.permissions.editMembers && (
						<SidebarNavSubItem end href={urlForSubpage(organization.name)}>
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
					{organization.permissions.editOrganization && (
						<SidebarNavSubItem
							href={urlForSubpage(organization.name, "settings")}
						>
							Settings
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
