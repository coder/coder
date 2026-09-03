import {
	ActivityIcon,
	BoxIcon,
	BuildingIcon,
	FileTextIcon,
	PanelLeftIcon,
	SparklesIcon,
} from "lucide-react";
import {
	type FC,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";
import { Link, NavLink, useLocation } from "react-router";
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
import { SidebarTopLevelNavItem } from "./SidebarTopLevelNavItem";

const DEFAULT_OPEN_SECTIONS_STORAGE_KEY = "admin-sidebar-open-sections";
const DEFAULT_OPEN_SECTIONS = ["deployment", "deployment-general"];

function readOpenSections(key: string): Set<string> | undefined {
	try {
		const raw = localStorage.getItem(key);
		if (!raw) {
			return undefined;
		}
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) {
			return undefined;
		}
		return new Set(
			parsed.filter((value): value is string => typeof value === "string"),
		);
	} catch {
		return undefined;
	}
}

function persistOpenSections(key: string, sections: Set<string>): void {
	try {
		localStorage.setItem(key, JSON.stringify([...sections]));
	} catch {
		// Silently ignore write failures.
	}
}

/**
 * Tracks which accordions are open. State is fully manual and
 * persisted; the only automatic change is opening the chain that
 * contains the current route so the active link is always visible.
 * Nothing is ever closed automatically.
 */
function useOpenSections(storageKey: string, activeChain: string[]) {
	const [openSections, setOpenSections] = useState<Set<string>>(
		() => readOpenSections(storageKey) ?? new Set(DEFAULT_OPEN_SECTIONS),
	);

	const chainKey = activeChain.join(",");
	useEffect(() => {
		const chain = chainKey ? chainKey.split(",") : [];
		if (chain.length === 0) {
			return;
		}
		setOpenSections((prev) => {
			if (chain.every((key) => prev.has(key))) {
				return prev;
			}
			const next = new Set(prev);
			for (const key of chain) {
				next.add(key);
			}
			persistOpenSections(storageKey, next);
			return next;
		});
	}, [chainKey, storageKey]);

	const toggleSection = useCallback(
		(key: string) => {
			setOpenSections((prev) => {
				const next = new Set(prev);
				if (next.has(key)) {
					next.delete(key);
				} else {
					next.add(key);
				}
				persistOpenSections(storageKey, next);
				return next;
			});
		},
		[storageKey],
	);

	return { openSections, toggleSection };
}

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

interface AdminNavLinkProps {
	href: string;
	children: ReactNode;
	/** Match the route exactly instead of by prefix. */
	end?: boolean;
	/**
	 * Items under a nested accordion use tighter 32px rows; items directly
	 * under an icon section keep 40px rows.
	 */
	nested?: boolean;
	/** Overrides NavLink matching for pages reachable from several URLs. */
	activeOverride?: boolean;
}

const AdminNavLink: FC<AdminNavLinkProps> = ({
	href,
	children,
	end,
	nested = false,
	activeOverride,
}) => {
	const sizeClass = nested ? "h-8 px-2" : "h-10 px-2 -mx-2";
	const baseClass = cn(
		"flex items-center rounded-md text-sm font-medium text-content-secondary no-underline hover:bg-surface-secondary transition-colors",
		sizeClass,
	);
	const activeClass = "font-semibold text-content-primary";

	if (activeOverride !== undefined) {
		return (
			<Link
				to={href}
				aria-current={activeOverride ? "page" : undefined}
				className={cn(baseClass, activeOverride && activeClass)}
			>
				{children}
			</Link>
		);
	}

	return (
		<NavLink
			to={href}
			end={end}
			className={({ isActive }) => cn(baseClass, isActive && activeClass)}
		>
			{children}
		</NavLink>
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
	/** Overridable so stories do not share persisted accordion state. */
	openSectionsStorageKey?: string;
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
	openSectionsStorageKey = DEFAULT_OPEN_SECTIONS_STORAGE_KEY,
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
		openSectionsStorageKey,
		activeChain,
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
				<AdminNavLink
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
				</AdminNavLink>
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
					<div className="py-1">
						<OrganizationSwitcher
							activeOrganization={activeOrganization}
							organizations={organizations}
							canCreateOrganization={permissions.createOrganization}
						/>
					</div>
					{orgBase && orgPermissions && (
						<>
							<AdminNavLink end href={orgBase}>
								Members
							</AdminNavLink>
							{orgPermissions.viewGroups && (
								<AdminNavLink href={`${orgBase}/groups`}>Groups</AdminNavLink>
							)}
							{orgPermissions.viewOrgRoles && (
								<AdminNavLink href={`${orgBase}/roles`}>Roles</AdminNavLink>
							)}
							{orgPermissions.viewProvisioners &&
								orgPermissions.viewProvisionerJobs && (
									<SidebarAccordion
										label="Provisioners"
										open={openSections.has("organizations-provisioners")}
										onToggle={() => toggleSection("organizations-provisioners")}
										active={activeChain.includes("organizations-provisioners")}
									>
										<AdminNavLink nested href={`${orgBase}/provisioners`}>
											Daemons
										</AdminNavLink>
										<AdminNavLink nested href={`${orgBase}/provisioner-keys`}>
											Keys
										</AdminNavLink>
										<AdminNavLink nested href={`${orgBase}/provisioner-jobs`}>
											Jobs
										</AdminNavLink>
									</SidebarAccordion>
								)}
							{orgPermissions.viewIdpSyncSettings && (
								<AdminNavLink href={`${orgBase}/idp-sync`}>
									IdP sync
								</AdminNavLink>
							)}
							{orgPermissions.editSettings && (
								<AdminNavLink href={`${orgBase}/settings`}>
									Settings
								</AdminNavLink>
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
