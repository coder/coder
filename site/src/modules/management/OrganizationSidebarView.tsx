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
	SettingsSidebarNavItem,
} from "components/Sidebar/Sidebar";
import type { Permissions } from "contexts/auth/permissions";
import { Check, ChevronDown, Plus } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, useState } from "react";
import { useNavigate } from "react-router-dom";

export interface OrganizationWithPermissions extends Organization {
	permissions: AuthorizationResponse;
}

interface SidebarProps {
	/** The active org name, if any.  Overrides activeSettings. */
	activeOrganization: OrganizationWithPermissions | undefined;
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
	return [`/organizations/${organizationName}`, subpage]
		.filter(Boolean)
		.join("/");
}

interface OrganizationsSettingsNavigationProps {
	/** The active org name if an org is being viewed. */
	activeOrganization: OrganizationWithPermissions | undefined;
	/** Organizations and their permissions or undefined if still fetching. */
	organizations: OrganizationWithPermissions[] | undefined;
	/** Site-wide permissions. */
	permissions: Permissions;
}

/**
 * Displays navigation items for the active organization and a combobox to
 * switch between organizations.
 *
 * If organizations or their permissions are still loading, show a loader.
 */
const OrganizationsSettingsNavigation: FC<
	OrganizationsSettingsNavigationProps
> = ({ activeOrganization, organizations, permissions }) => {
	// Wait for organizations and their permissions to load
	if (!organizations || !activeOrganization) {
		return <Loader />;
	}

	const sortedOrganizations = [...organizations].sort((a, b) => {
		// active org first
		if (a.id === activeOrganization.id) return -1;
		if (b.id === activeOrganization.id) return 1;

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
							{activeOrganization && (
								<Avatar
									size="sm"
									src={activeOrganization.icon}
									fallback={activeOrganization.display_name}
								/>
							)}
							<span className="truncate">
								{activeOrganization?.display_name || activeOrganization?.name}
							</span>
						</div>
						<ChevronDown />
					</Button>
				</PopoverTrigger>
				<PopoverContent align="start" className="w-60">
					<Command loop>
						<CommandList>
							<CommandGroup className="pb-2">
								{sortedOrganizations.length > 1 && (
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
												{activeOrganization.name === organization.name && (
													<Check
														size={16}
														strokeWidth={2}
														className="ml-auto"
													/>
												)}
											</CommandItem>
										))}
									</div>
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
			<OrganizationSettingsNavigation
				key={activeOrganization.id}
				organization={activeOrganization}
			/>
		</>
	);
};

interface OrganizationSettingsNavigationProps {
	organization: OrganizationWithPermissions;
}

const OrganizationSettingsNavigation: FC<
	OrganizationSettingsNavigationProps
> = ({ organization }) => {
	return (
		<>
			<div className="flex flex-col gap-1 my-2">
				{organization.permissions.editMembers && (
					<SettingsSidebarNavItem end href={urlForSubpage(organization.name)}>
						Members
					</SettingsSidebarNavItem>
				)}
				{organization.permissions.editGroups && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "groups")}
					>
						Groups
					</SettingsSidebarNavItem>
				)}
				{organization.permissions.assignOrgRole && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "roles")}
					>
						Roles
					</SettingsSidebarNavItem>
				)}
				{organization.permissions.viewProvisioners && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "provisioners")}
					>
						Provisioners
					</SettingsSidebarNavItem>
				)}
				{organization.permissions.viewIdpSyncSettings && (
					<SettingsSidebarNavItem
						href={urlForSubpage(organization.name, "idp-sync")}
					>
						IdP Sync
					</SettingsSidebarNavItem>
				)}
				{organization.permissions.editOrganization && (
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
