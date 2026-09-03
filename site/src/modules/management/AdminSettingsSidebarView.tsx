import {
	ActivityIcon,
	BoxIcon,
	BuildingIcon,
	FileTextIcon,
	PanelLeftIcon,
	SparklesIcon,
} from "lucide-react";
import type { FC } from "react";
import { useLocation } from "react-router";
import type {
	BuildInfoResponse,
	Experiment,
	Organization,
} from "#/api/typesGenerated";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	canViewDeploymentSettings,
	type Permissions,
} from "#/modules/permissions";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";
import { cn } from "#/utils/cn";
import {
	type AdminNavItem,
	type AdminNavSection,
	adminSectionChainForRoute,
	aiCoderAgentsSection,
	aiNavItems,
	canViewAISettings,
	deploymentNavSections,
	logsNavItems,
} from "./adminNavigation";
import { OrganizationSwitcher } from "./OrganizationSwitcher";
import { SidebarNavLink } from "./SidebarNavLink";
import { SidebarTopLevelNavItem } from "./SidebarTopLevelNavItem";
import { useOpenSections } from "./useOpenSections";

const DEFAULT_OPEN_SECTIONS = ["deployment", "deployment-general"];

/**
 * Pinned header for the admin sidebar: the section title and the
 * collapse toggle, over a full-bleed divider. Rendered by the layout
 * through CollapsibleSidebar's header slot so it never scrolls.
 */
export const AdminSettingsSidebarHeader: FC = () => {
	const { collapsed, toggle } = useSidebarContext();

	return (
		<>
			<div className="px-3 py-3">
				<button
					type="button"
					onClick={toggle}
					aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
					className={cn(
						"group flex items-center bg-transparent border-none cursor-pointer p-0",
						collapsed
							? "w-10 h-10 justify-center rounded-md"
							: "w-full px-1 rounded-md h-10",
					)}
				>
					{!collapsed && (
						<span className="text-sm text-content-secondary">
							Admin settings
						</span>
					)}
					<PanelLeftIcon
						className={cn(
							"size-4 text-content-secondary group-hover:text-content-primary transition-colors",
							!collapsed && "ml-auto",
						)}
					/>
				</button>
			</div>
			<div className="h-px shrink-0 bg-border" />
		</>
	);
};

interface AdminSettingsSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
	/** Licensed, non-trial deployments hide the Trial Upgrade link. */
	hidePremiumTab: boolean;
	experiments: Experiment[];
	buildInfo: BuildInfoResponse;
	/** Whether the Organizations section is shown at all. */
	canViewOrganizations: boolean;
	/** Organizations the user can view, for the switcher. */
	organizations: readonly Organization[];
	/** The organization whose links are listed. */
	activeOrganization: Organization | undefined;
	/** Permissions for the active organization, undefined while loading. */
	orgPermissions: OrganizationPermissions | undefined;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewAIBridge: boolean;
	/** The user can manage chat models in at least one organization. */
	canAccessOrganizationModels: boolean;
	/** The user can share MCP servers in at least one organization. */
	canShareOrganizationMCPServers: boolean;
	/** Sections open on first render; overrides the route-derived default. */
	initialOpenSections?: string[];
}

/**
 * Unified navigation for every admin settings area: deployment,
 * organizations, AI, logs, and healthcheck. Section headers are pure
 * expand/collapse toggles; only leaf links navigate.
 */
export const AdminSettingsSidebarView: FC<AdminSettingsSidebarViewProps> = ({
	permissions,
	hidePremiumTab,
	experiments,
	buildInfo,
	canViewOrganizations,
	organizations,
	activeOrganization,
	orgPermissions,
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
	canAccessOrganizationModels,
	canShareOrganizationMCPServers,
	initialOpenSections,
}) => {
	const { pathname } = useLocation();

	const deploymentSections = deploymentNavSections({
		permissions,
		hidePremiumTab,
		experiments,
		buildInfo,
	});
	const aiAccess = {
		canAccessOrganizationModels,
		canShareOrganizationMCPServers,
	};
	const activeChain = adminSectionChainForRoute(pathname, deploymentSections);
	const { openSections, toggleSection } = useOpenSections(
		activeChain,
		DEFAULT_OPEN_SECTIONS,
		initialOpenSections,
	);

	const logsItems = logsNavItems({
		canViewAuditLog,
		canViewConnectionLog,
		canViewAIBridge,
	});

	const renderItems = (items: AdminNavItem[], nested = false) =>
		items
			.filter((item) => item.visible)
			.map((item) => (
				<SidebarNavLink
					key={item.href}
					href={item.href}
					end={item.end}
					nested={nested}
					activeOverride={
						item.activePrefixes
							? item.activePrefixes.some(
									(prefix) =>
										pathname === prefix || pathname.startsWith(`${prefix}/`),
								)
							: undefined
					}
				>
					{item.label}
				</SidebarNavLink>
			));

	const renderNestedSection = (section: AdminNavSection) =>
		section.items.some((item) => item.visible) && (
			<SidebarAccordion
				key={section.key}
				label={section.label}
				open={openSections.has(section.key)}
				onToggle={() => toggleSection(section.key)}
				active={activeChain.includes(section.key)}
			>
				{renderItems(section.items, true)}
			</SidebarAccordion>
		);

	const orgBase = activeOrganization
		? `/organizations/${activeOrganization.name}`
		: undefined;

	return (
		<div className="flex flex-col gap-3">
			{canViewDeploymentSettings(permissions) && (
				<SidebarAccordion
					icon={BoxIcon}
					label="Deployment"
					open={openSections.has("deployment")}
					onToggle={() => toggleSection("deployment")}
					active={activeChain[0] === "deployment"}
				>
					{deploymentSections.map(renderNestedSection)}
				</SidebarAccordion>
			)}

			{canViewOrganizations && (
				<SidebarAccordion
					icon={BuildingIcon}
					label="Organizations"
					open={openSections.has("organizations")}
					onToggle={() => toggleSection("organizations")}
					active={activeChain[0] === "organizations"}
				>
					<div className="py-1 -ml-7 -mr-1">
						<OrganizationSwitcher
							activeOrganization={activeOrganization}
							organizations={organizations}
							canCreateOrganization={permissions.createOrganization}
						/>
					</div>
					{orgBase && orgPermissions && (
						<>
							<SidebarNavLink end href={orgBase}>
								Members
							</SidebarNavLink>
							{orgPermissions.viewGroups && (
								<SidebarNavLink href={`${orgBase}/groups`}>
									Groups
								</SidebarNavLink>
							)}
							{orgPermissions.viewOrgRoles && (
								<SidebarNavLink href={`${orgBase}/roles`}>Roles</SidebarNavLink>
							)}
							{orgPermissions.viewProvisioners &&
								orgPermissions.viewProvisionerJobs && (
									<SidebarAccordion
										label="Provisioners"
										open={openSections.has("organizations-provisioners")}
										onToggle={() => toggleSection("organizations-provisioners")}
										active={activeChain.includes("organizations-provisioners")}
									>
										<SidebarNavLink nested href={`${orgBase}/provisioners`}>
											Daemons
										</SidebarNavLink>
										<SidebarNavLink nested href={`${orgBase}/provisioner-keys`}>
											Keys
										</SidebarNavLink>
										<SidebarNavLink nested href={`${orgBase}/provisioner-jobs`}>
											Jobs
										</SidebarNavLink>
									</SidebarAccordion>
								)}
							{orgPermissions.viewIdpSyncSettings && (
								<SidebarNavLink href={`${orgBase}/idp-sync`}>
									IdP sync
								</SidebarNavLink>
							)}
							{orgPermissions.editSettings && (
								<SidebarNavLink href={`${orgBase}/settings`}>
									Settings
								</SidebarNavLink>
							)}
						</>
					)}
				</SidebarAccordion>
			)}

			{canViewAISettings(permissions, aiAccess) && (
				<SidebarAccordion
					icon={SparklesIcon}
					label="AI"
					open={openSections.has("ai")}
					onToggle={() => toggleSection("ai")}
					active={activeChain[0] === "ai"}
				>
					{renderItems(aiNavItems(permissions, aiAccess))}
					{renderNestedSection(aiCoderAgentsSection(permissions, aiAccess))}
				</SidebarAccordion>
			)}

			{logsItems.some((item) => item.visible) && (
				<SidebarAccordion
					icon={FileTextIcon}
					label="Logs"
					open={openSections.has("logs")}
					onToggle={() => toggleSection("logs")}
					active={activeChain[0] === "logs"}
				>
					{renderItems(logsItems)}
				</SidebarAccordion>
			)}

			{permissions.viewDebugInfo && (
				<SidebarTopLevelNavItem
					label="Healthcheck"
					href="/health"
					icon={ActivityIcon}
					active={pathname.startsWith("/health")}
				/>
			)}
		</div>
	);
};
