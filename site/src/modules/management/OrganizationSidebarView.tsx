import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import {
	Command,
	CommandGroup,
	CommandItem,
	CommandList,
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

interface SidebarProps {
	/** The active org name, if any.  Overrides activeSettings. */
	activeOrganization: Organization | undefined;
	/** Permissions for the active organization */
	orgPermissions: OrganizationPermissions | undefined;
	/** Organizations and their permissions or undefined if still fetching. */
	organizations: readonly Organization[];
	/** Site-wide permissions. */
	permissions: Permissions;
}

/**
 * Organization settings left sidebar menu.
 */
export const OrganizationSidebarView: FC<SidebarProps> = ({
	activeOrganization,
	orgPermissions,
	organizations,
	permissions,
}) => {
	// Sort organizations to put active organization first
	const sortedOrganizations = activeOrganization
		? [
				activeOrganization,
				...organizations.filter((org) => org.id !== activeOrganization.id),
			]
		: organizations;

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
						<CommandList>
							<CommandGroup className="pb-2">
								{sortedOrganizations.length > (activeOrganization ? 1 : 0) ? (
									<div className="flex flex-col max-h-[260px] overflow-y-auto">
										{sortedOrganizations.map((organization) => (
											<CommandItem
												key={organization.id}
												value={organization.name}
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
													<Check
														size={16}
														strokeWidth={2}
														className="ml-auto"
													/>
												)}
											</CommandItem>
										))}
									</div>
								) : (
									!permissions.createOrganization && (
										<span className="select-none text-content-disabled text-center rounded-sm px-2 py-2 text-sm font-medium">
											No more organizations
										</span>
									)
								)}
								{permissions.createOrganization && (
									<>
										{organizations.length > 1 && (
											<hr className="h-px my-2 border-none bg-border -mx-2" />
										)}
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
									</>
								)}
							</CommandGroup>
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
	orgPermissions: AuthorizationResponse;
}

const OrganizationSettingsNavigation: FC<
	OrganizationSettingsNavigationProps
> = ({ organization, orgPermissions }) => {
	return (
		<>
			<div className="flex flex-col gap-1 my-2">
				{orgPermissions.viewMembers && (
					<SettingsSidebarNavItem end href={urlForSubpage(organization.name)}>
						Members
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.viewGroups && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "groups")}
					>
						Groups
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.assignOrgRole && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "roles")}
					>
						Roles
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.viewProvisioners && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "provisioners")}
					>
						Provisioners
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.viewIdpSyncSettings && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "idp-sync")}
					>
						IdP Sync
					</SettingsSidebarNavItem>
				)}
				{orgPermissions.editOrganization && (
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
