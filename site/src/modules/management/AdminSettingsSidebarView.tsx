import {
	ActivityIcon,
	BoxIcon,
	BuildingIcon,
	FileTextIcon,
	PanelLeftIcon,
	SparklesIcon,
} from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import { useLocation } from "react-router";
import type {
	BuildInfoResponse,
	Experiment,
	Organization,
} from "#/api/typesGenerated";
import { SettingsSidebarNavItem } from "#/components/Sidebar/Sidebar";
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

interface AdminSettingsSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
	hasPremiumLicense: boolean;
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
	hasPremiumLicense,
	experiments,
	buildInfo,
	canViewOrganizations,
	organizations,
	activeOrganization,
	orgPermissions,
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
	openSectionsStorageKey = DEFAULT_OPEN_SECTIONS_STORAGE_KEY,
}) => {
	const { collapsed, toggle } = useSidebarContext();
	const { pathname } = useLocation();

	const deploymentSections = deploymentNavSections({
		permissions,
		hasPremiumLicense,
		experiments,
		buildInfo,
	});
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

	const renderItems = (items: AdminNavItem[]) =>
		items
			.filter((item) => item.visible)
			.map((item) => (
				<SettingsSidebarNavItem key={item.href} href={item.href} end={item.end}>
					{item.label}
				</SettingsSidebarNavItem>
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
				{renderItems(section.items)}
			</SidebarAccordion>
		);

	const orgBase = activeOrganization
		? `/organizations/${activeOrganization.name}`
		: undefined;

	return (
		<div className="flex flex-col gap-1">
			<button
				type="button"
				onClick={toggle}
				className={cn(
					"group flex items-center bg-transparent border-none cursor-pointer p-0 my-3",
					collapsed
						? "w-10 h-10 justify-center rounded-md"
						: "w-full px-3 rounded-md h-10",
				)}
			>
				{!collapsed && (
					<span className="text-sm text-content-secondary">Admin settings</span>
				)}
				<PanelLeftIcon
					className={cn(
						"size-4 text-content-secondary group-hover:text-content-primary transition-colors",
						!collapsed && "ml-auto",
					)}
				/>
			</button>
			{/* Full-bleed divider: cancels the nav's horizontal padding. */}
			<div className="h-px bg-border -mx-3 mb-3" />

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
							<SettingsSidebarNavItem end href={orgBase}>
								Members
							</SettingsSidebarNavItem>
							{orgPermissions.viewGroups && (
								<SettingsSidebarNavItem href={`${orgBase}/groups`}>
									Groups
								</SettingsSidebarNavItem>
							)}
							{orgPermissions.viewOrgRoles && (
								<SettingsSidebarNavItem href={`${orgBase}/roles`}>
									Roles
								</SettingsSidebarNavItem>
							)}
							{orgPermissions.viewProvisioners &&
								orgPermissions.viewProvisionerJobs && (
									<SidebarAccordion
										label="Provisioners"
										open={openSections.has("organizations-provisioners")}
										onToggle={() => toggleSection("organizations-provisioners")}
										active={activeChain.includes("organizations-provisioners")}
									>
										<SettingsSidebarNavItem href={`${orgBase}/provisioners`}>
											Daemons
										</SettingsSidebarNavItem>
										<SettingsSidebarNavItem
											href={`${orgBase}/provisioner-keys`}
										>
											Keys
										</SettingsSidebarNavItem>
										<SettingsSidebarNavItem
											href={`${orgBase}/provisioner-jobs`}
										>
											Jobs
										</SettingsSidebarNavItem>
									</SidebarAccordion>
								)}
							{orgPermissions.viewIdpSyncSettings && (
								<SettingsSidebarNavItem href={`${orgBase}/idp-sync`}>
									IdP sync
								</SettingsSidebarNavItem>
							)}
							{orgPermissions.editSettings && (
								<SettingsSidebarNavItem href={`${orgBase}/settings`}>
									Settings
								</SettingsSidebarNavItem>
							)}
						</>
					)}
				</SidebarAccordion>
			)}

			{canViewAISettings(permissions) && (
				<SidebarAccordion
					icon={SparklesIcon}
					label="AI"
					open={openSections.has("ai")}
					onToggle={() => toggleSection("ai")}
					active={activeChain[0] === "ai"}
				>
					{renderItems(aiNavItems(permissions))}
					{renderNestedSection(aiCoderAgentsSection(permissions))}
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
