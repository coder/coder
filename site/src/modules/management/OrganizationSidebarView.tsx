import type { Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
	CommandSeparator,
} from "components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { SettingsSidebarNavItem } from "components/Sidebar/Sidebar";
import type { Permissions } from "contexts/auth/permissions";
import { Check, ChevronDown, Plus } from "lucide-react";
import { type FC, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { OrganizationPermissions } from "./organizationPermissions";

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
						className="w-60 justify-between p-2 h-11"
					>
						<div className="flex flex-row gap-2 items-center p-2 truncate">
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
						</div>
						<ChevronDown />
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
												<Check size={16} strokeWidth={2} className="ml-auto" />
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
											<Plus /> Create Organization
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
		<>
			<div className="flex flex-col gap-1 my-2">
				<SettingsSidebarNavItem end href={urlForSubpage(organization.name)}>
					Members
				</SettingsSidebarNavItem>
				{orgPermissions.viewGroups && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "groups")}
					>
						Groups
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.viewOrgRoles && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "roles")}
					>
						Roles
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.viewProvisioners &&
					orgPermissions.viewProvisionerJobs && (
						<>
							<SettingsSidebarNavItem
								href={urlForSubpage(organization.name, "provisioners")}
							>
								Provisioners
							</SettingsSidebarNavItem>
							<SettingsSidebarNavItem
								href={urlForSubpage(organization.name, "provisioner-jobs")}
							>
								Provisioner Jobs
							</SettingsSidebarNavItem>
						</>
					)}
				{orgPermissions.viewIdpSyncSettings && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "idp-sync")}
					>
						IdP Sync
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.editSettings && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "settings")}
					>
						Settings
					</SettingsSidebarNavItem>
				)}
			</div>
		</>
	);
};
