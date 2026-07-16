import {
	CheckIcon,
	FolderSyncIcon,
	HammerIcon,
	KeyIcon,
	ListChecksIcon,
	PanelLeftIcon,
	PlusIcon,
	SettingsIcon,
	ShieldIcon,
	UsersIcon,
	UsersRoundIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { NavLink, useNavigate } from "react-router";
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

interface CollapsedNavItemProps {
	label: string;
	href: string;
	icon: FC<{ className?: string }>;
	end?: boolean;
}

/**
 * Icon-only nav link with a tooltip for collapsed mode. Clicking it
 * navigates and re-expands the sidebar, matching the other push-mode
 * settings sections.
 */
const CollapsedNavItem: FC<CollapsedNavItemProps> = ({
	label,
	href,
	icon: Icon,
	end,
}) => {
	const { expand } = useSidebarContext();

	return (
		<TooltipProvider>
			<Tooltip delayDuration={0}>
				<TooltipTrigger asChild>
					<NavLink
						end={end}
						to={href}
						onClick={expand}
						className="flex items-center justify-center w-10 h-10 rounded-md no-underline hover:bg-surface-secondary"
					>
						{({ isActive }) => (
							<Icon
								className={cn(
									"size-4 flex-shrink-0 text-content-secondary",
									isActive && "text-content-primary",
								)}
							/>
						)}
					</NavLink>
				</TooltipTrigger>
				<TooltipContent side="right">{label}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

interface OrganizationSettingsNavigationProps {
	organization: Organization;
	orgPermissions: OrganizationPermissions;
}

const OrganizationSettingsNavigation: FC<
	OrganizationSettingsNavigationProps
> = ({ organization, orgPermissions }) => {
	const { collapsed } = useSidebarContext();

	const navItems = [
		{
			label: "Members",
			href: urlForSubpage(organization.name),
			icon: UsersIcon,
			end: true,
			visible: true,
		},
		{
			label: "Groups",
			href: urlForSubpage(organization.name, "groups"),
			icon: UsersRoundIcon,
			visible: orgPermissions.viewGroups,
		},
		{
			label: "Roles",
			href: urlForSubpage(organization.name, "roles"),
			icon: ShieldIcon,
			visible: orgPermissions.viewOrgRoles,
		},
		{
			label: "Provisioners",
			href: urlForSubpage(organization.name, "provisioners"),
			icon: HammerIcon,
			visible:
				orgPermissions.viewProvisioners && orgPermissions.viewProvisionerJobs,
		},
		{
			label: "Provisioner Keys",
			href: urlForSubpage(organization.name, "provisioner-keys"),
			icon: KeyIcon,
			visible:
				orgPermissions.viewProvisioners && orgPermissions.viewProvisionerJobs,
		},
		{
			label: "Provisioner Jobs",
			href: urlForSubpage(organization.name, "provisioner-jobs"),
			icon: ListChecksIcon,
			visible:
				orgPermissions.viewProvisioners && orgPermissions.viewProvisionerJobs,
		},
		{
			label: "IdP Sync",
			href: urlForSubpage(organization.name, "idp-sync"),
			icon: FolderSyncIcon,
			visible: orgPermissions.viewIdpSyncSettings,
		},
		{
			label: "Settings",
			href: urlForSubpage(organization.name, "settings"),
			icon: SettingsIcon,
			visible: orgPermissions.editSettings,
		},
	];

	return (
		<div className="flex flex-col gap-1 my-2">
			{navItems
				.filter((item) => item.visible)
				.map((item) =>
					collapsed ? (
						<CollapsedNavItem
							key={item.href}
							label={item.label}
							href={item.href}
							icon={item.icon}
							end={item.end}
						/>
					) : (
						<SettingsSidebarNavItem
							key={item.href}
							href={item.href}
							end={item.end}
						>
							{item.label}
						</SettingsSidebarNavItem>
					),
				)}
		</div>
	);
};
