import {
	CheckIcon,
	NetworkIcon,
	PlusIcon,
	RepeatIcon,
	SettingsIcon,
	ShieldIcon,
	UserIcon,
	WebhookIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { useNavigate } from "react-router";
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
import { SidebarGroup, SidebarNavItem } from "#/components/Sidebar";
import type { Permissions } from "#/modules/permissions";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";

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
		<>
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
				<PopoverContent
					align="start"
					className="w-[var(--radix-popover-trigger-width)]"
				>
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
			{activeOrganization && orgPermissions && (
				<OrganizationSettingsNavigation
					key={activeOrganization.id}
					organization={activeOrganization}
					orgPermissions={orgPermissions}
				/>
			)}
		</>
	);
};

function urlForSubpage(organizationName: string, subpage = ""): string {
	return [`/organizations/${organizationName}`, subpage]
		.filter(Boolean)
		.join("/");
}

interface OrganizationSettingsNavigationProps {
	organization: Organization;
	orgPermissions: OrganizationPermissions;
}

const OrganizationSettingsNavigation: FC<
	OrganizationSettingsNavigationProps
> = ({ organization, orgPermissions }) => {
	return (
		<div className="flex flex-col gap-1 my-2">
			<SidebarNavItem
				end
				icon={UserIcon}
				href={urlForSubpage(organization.name)}
			>
				Members
			</SidebarNavItem>
			{orgPermissions.viewGroups && (
				<SidebarNavItem
					icon={NetworkIcon}
					href={urlForSubpage(organization.name, "groups")}
				>
					Groups
				</SidebarNavItem>
			)}
			{orgPermissions.viewOrgRoles && (
				<SidebarNavItem
					icon={ShieldIcon}
					href={urlForSubpage(organization.name, "roles")}
				>
					Roles
				</SidebarNavItem>
			)}
			{orgPermissions.viewProvisioners &&
				orgPermissions.viewProvisionerJobs && (
					<SidebarGroup
						icon={WebhookIcon}
						label="Provisioners"
						href={urlForSubpage(organization.name, "provisioners")}
					>
						<SidebarNavItem
							end
							href={urlForSubpage(organization.name, "provisioners")}
						>
							Overview
						</SidebarNavItem>
						<SidebarNavItem
							href={urlForSubpage(organization.name, "provisioner-keys")}
						>
							Keys
						</SidebarNavItem>
						<SidebarNavItem
							href={urlForSubpage(organization.name, "provisioner-jobs")}
						>
							Jobs
						</SidebarNavItem>
					</SidebarGroup>
				)}
			{orgPermissions.viewIdpSyncSettings && (
				<SidebarNavItem
					icon={RepeatIcon}
					href={urlForSubpage(organization.name, "idp-sync")}
				>
					IdP sync
				</SidebarNavItem>
			)}
			{orgPermissions.editSettings && (
				<SidebarNavItem
					icon={SettingsIcon}
					href={urlForSubpage(organization.name, "settings")}
				>
					Settings
				</SidebarNavItem>
			)}
		</div>
	);
};
