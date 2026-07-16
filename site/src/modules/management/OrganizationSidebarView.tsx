import {
	CheckIcon,
	NetworkIcon,
	PanelLeftIcon,
	PlusIcon,
	RepeatIcon,
	SettingsIcon,
	ShieldIcon,
	UserIcon,
	WebhookIcon,
} from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Organization } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
	CommandSeparator,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { SettingsSidebarNavItem } from "#/components/Sidebar/Sidebar";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { Permissions } from "#/modules/permissions";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";
import { cn } from "#/utils/cn";
import { SidebarTopLevelNavItem } from "./SidebarTopLevelNavItem";

interface OrganizationsSettingsNavigationProps {
	/** The organization selected from the dropdown */
	activeOrganization: Organization | undefined;
	/** Permissions for the active organization */
	orgPermissions: OrganizationPermissions | undefined;
	/** Organizations and their permissions or undefined if still fetching. */
	organizations: readonly Organization[];
	/** Site-wide permissions. */
	permissions: Permissions;
}

/**
 * Displays navigation items for the active organization and a combobox to
 * switch between organizations.
 *
 * If organizations or their permissions are still loading, show a loader.
 */
export const OrganizationSidebarView: FC<
	OrganizationsSettingsNavigationProps
> = ({ activeOrganization, orgPermissions, organizations, permissions }) => {
	const { collapsed, expand, toggle } = useSidebarContext();
	const sortedOrganizations = [...organizations].sort((a, b) => {
		// active org first
		if (a.id === activeOrganization?.id) return -1;
		if (b.id === activeOrganization?.id) return 1;

		return a.display_name
			.toLowerCase()
			.localeCompare(b.display_name.toLowerCase());
	});

	const [isPopoverOpen, setIsPopoverOpen] = useState(false);
	const navigate = useNavigate();

	return (
		<div className="flex flex-col gap-1">
			<button
				type="button"
				onClick={toggle}
				className={cn(
					"group flex items-center bg-transparent border-none cursor-pointer mb-1 p-0",
					collapsed
						? "w-10 h-10 justify-center rounded-md"
						: "w-full px-3 rounded-md h-10",
				)}
			>
				{!collapsed && (
					<span className="text-sm text-content-secondary">Organizations</span>
				)}
				<PanelLeftIcon
					className={cn(
						"size-4 text-content-secondary group-hover:text-content-primary transition-colors",
						!collapsed && "ml-auto",
					)}
				/>
			</button>

			{collapsed ? (
				activeOrganization && (
					<TooltipProvider>
						<Tooltip delayDuration={0}>
							<TooltipTrigger asChild>
								{/* The full combobox does not fit the icon rail, so
								    expand the sidebar to let the user switch orgs. */}
								<button
									type="button"
									onClick={expand}
									className="flex items-center justify-center w-10 h-10 rounded-md cursor-pointer bg-transparent border-none hover:bg-surface-secondary"
								>
									<Avatar
										size="sm"
										src={activeOrganization.icon}
										fallback={activeOrganization.display_name}
									/>
								</button>
							</TooltipTrigger>
							<TooltipContent side="right">
								{activeOrganization.display_name || activeOrganization.name}
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				)
			) : (
				<Popover open={isPopoverOpen} onOpenChange={setIsPopoverOpen}>
					<PopoverTrigger asChild>
						<Button
							variant="outline"
							aria-expanded={isPopoverOpen}
							className="w-full gap-2 justify-start"
						>
							{activeOrganization ? (
								<>
									<Avatar
										size="sm"
										src={activeOrganization.icon}
										fallback={activeOrganization.display_name}
									/>
									<span className="truncate">
										{activeOrganization.display_name || activeOrganization.name}
									</span>
								</>
							) : (
								<span className="truncate">No organization selected</span>
							)}
							<ChevronDownIcon className="ml-auto !size-icon-sm" />
						</Button>
					</PopoverTrigger>
					<PopoverContent align="start" className="w-60">
						<Command loop>
							<CommandInput placeholder="Find organization" />
							<CommandList>
								<CommandEmpty>No organization found.</CommandEmpty>
								<CommandGroup className="pb-2">
									<div className="flex flex-col max-h-[260px] overflow-y-auto">
										{sortedOrganizations.map((organization) => (
											<CommandItem
												key={organization.id}
												value={`${organization.display_name} ${organization.name}`}
												onSelect={() => {
													setIsPopoverOpen(false);
													navigate(urlForSubpage(organization.name));
												}}
												// There is currently an issue with the cmdk component for keyboard navigation
												// https://github.com/pacocoursey/cmdk/issues/322
												tabIndex={0}
											>
												<Avatar
													size="sm"
													src={organization.icon}
													fallback={organization.display_name}
												/>
												<span className="truncate">
													{organization?.display_name || organization?.name}
												</span>
												{activeOrganization?.name === organization.name && (
													<CheckIcon className="ml-auto" />
												)}
											</CommandItem>
										))}
									</div>
								</CommandGroup>
								{permissions.createOrganization && (
									<>
										{organizations.length > 1 && <CommandSeparator />}
										<CommandGroup>
											<CommandItem
												className="flex justify-center data-[selected=true]:bg-transparent"
												onSelect={() => {
													setIsPopoverOpen(false);
													setTimeout(() => {
														navigate("/organizations/new");
													}, 200);
												}}
											>
												<PlusIcon /> Create Organization
											</CommandItem>
										</CommandGroup>
									</>
								)}
							</CommandList>
						</Command>
					</PopoverContent>
				</Popover>
			)}
			{activeOrganization && orgPermissions && (
				<OrganizationSettingsNavigation
					key={activeOrganization.id}
					organization={activeOrganization}
					orgPermissions={orgPermissions}
				/>
			)}
		</div>
	);
};

function urlForSubpage(organizationName: string, subpage = ""): string {
	return [`/organizations/${organizationName}`, subpage]
		.filter(Boolean)
		.join("/");
}

function isRouteActive(pathname: string, url: string, end = false): boolean {
	if (end) {
		return pathname === url;
	}
	return pathname === url || pathname.startsWith(`${url}/`);
}

interface OrganizationSettingsNavigationProps {
	organization: Organization;
	orgPermissions: OrganizationPermissions;
}

const OrganizationSettingsNavigation: FC<
	OrganizationSettingsNavigationProps
> = ({ organization, orgPermissions }) => {
	const { pathname } = useLocation();

	const membersUrl = urlForSubpage(organization.name);
	const groupsUrl = urlForSubpage(organization.name, "groups");
	const rolesUrl = urlForSubpage(organization.name, "roles");
	const provisionersUrl = urlForSubpage(organization.name, "provisioners");
	const provisionerKeysUrl = urlForSubpage(
		organization.name,
		"provisioner-keys",
	);
	const provisionerJobsUrl = urlForSubpage(
		organization.name,
		"provisioner-jobs",
	);
	const idpSyncUrl = urlForSubpage(organization.name, "idp-sync");
	const settingsUrl = urlForSubpage(organization.name, "settings");

	// The accordion owns the current route when any provisioner page
	// is open.
	const provisionersActive = [
		provisionersUrl,
		provisionerKeysUrl,
		provisionerJobsUrl,
	].some((url) => isRouteActive(pathname, url));

	const [provisionersOpen, setProvisionersOpen] = useState(provisionersActive);

	// When navigation changes the active route, open the accordion
	// only when one of its pages is active, close otherwise.
	useEffect(() => {
		setProvisionersOpen(provisionersActive);
	}, [provisionersActive]);

	const toggleProvisioners = useCallback(() => {
		setProvisionersOpen((prev) => !prev);
	}, []);

	return (
		<div className="flex flex-col gap-1 my-2">
			{orgPermissions.viewGroups && (
				<SidebarTopLevelNavItem
					label="Groups"
					href={groupsUrl}
					icon={NetworkIcon}
					active={isRouteActive(pathname, groupsUrl)}
				/>
			)}
			<SidebarTopLevelNavItem
				label="Members"
				href={membersUrl}
				icon={UserIcon}
				end
				active={isRouteActive(pathname, membersUrl, true)}
			/>
			{orgPermissions.viewOrgRoles && (
				<SidebarTopLevelNavItem
					label="Roles"
					href={rolesUrl}
					icon={ShieldIcon}
					active={isRouteActive(pathname, rolesUrl)}
				/>
			)}
			{orgPermissions.viewProvisioners &&
				orgPermissions.viewProvisionerJobs && (
					<SidebarAccordion
						icon={WebhookIcon}
						label="Provisioners"
						href={provisionersUrl}
						open={provisionersOpen}
						onToggle={toggleProvisioners}
						active={provisionersActive}
					>
						<div className="flex flex-col gap-1">
							<SettingsSidebarNavItem end href={provisionersUrl}>
								Provisioners
							</SettingsSidebarNavItem>
							<SettingsSidebarNavItem href={provisionerKeysUrl}>
								Keys
							</SettingsSidebarNavItem>
							<SettingsSidebarNavItem href={provisionerJobsUrl}>
								Jobs
							</SettingsSidebarNavItem>
						</div>
					</SidebarAccordion>
				)}
			{orgPermissions.viewIdpSyncSettings && (
				<SidebarTopLevelNavItem
					label="IdP sync"
					href={idpSyncUrl}
					icon={RepeatIcon}
					active={isRouteActive(pathname, idpSyncUrl)}
				/>
			)}
			{orgPermissions.editSettings && (
				<SidebarTopLevelNavItem
					label="Settings"
					href={settingsUrl}
					icon={SettingsIcon}
					active={isRouteActive(pathname, settingsUrl)}
				/>
			)}
		</div>
	);
};
