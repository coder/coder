import {
	ArrowUpRightIcon,
	HardDriveIcon,
	PanelLeftIcon,
	SettingsIcon,
	UserLockIcon,
} from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import type { BuildInfoResponse, Experiment } from "#/api/typesGenerated";
import { SettingsSidebarNavItem as SidebarNavItem } from "#/components/Sidebar/Sidebar";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import type { Permissions } from "#/modules/permissions";
import { getPrereleaseFlag } from "#/utils/buildInfo";
import { cn } from "#/utils/cn";
import type { DeploymentSection } from "./useActiveDeploymentSection";

interface DeploymentSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
	showOrganizations: boolean;
	hasPremiumLicense: boolean;
	experiments: Experiment[];
	buildInfo: BuildInfoResponse;
	/** Which accordion section is active based on the current route. */
	activeSection: DeploymentSection;
}

/**
 * Displays navigation for deployment settings grouped into accordion
 * sections. Section headers toggle freely, so multiple sections can be
 * open at once. Clicking a sub-item link collapses all other sections
 * so only the active section stays open after navigation.
 */
export const DeploymentSidebarView: FC<DeploymentSidebarViewProps> = ({
	permissions,
	showOrganizations,
	hasPremiumLicense,
	experiments,
	buildInfo,
	activeSection,
}) => {
	const { collapsed, toggle } = useSidebarContext();

	// Track which sections are open as a Set so multiple can be
	// expanded at the same time via the accordion headers.
	const [openSections, setOpenSections] = useState<Set<DeploymentSection>>(
		() => new Set([activeSection]),
	);

	// When a sub-item link is clicked (route changes), collapse
	// everything except the newly active section.
	useEffect(() => {
		setOpenSections(new Set([activeSection]));
	}, [activeSection]);

	const toggleSection = useCallback((section: DeploymentSection) => {
		setOpenSections((prev) => {
			const next = new Set(prev);
			if (next.has(section)) {
				next.delete(section);
			} else {
				next.add(section);
			}
			return next;
		});
	}, []);

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
					<span className="text-sm text-content-secondary">Deployment</span>
				)}
				<PanelLeftIcon
					className={cn(
						"size-4 text-content-secondary group-hover:text-content-primary transition-colors",
						!collapsed && "ml-auto",
					)}
				/>
			</button>

			<SidebarAccordion
				icon={SettingsIcon}
				label="General"
				href="/deployment/overview"
				open={openSections.has("general")}
				onToggle={() => toggleSection("general")}
				active={activeSection === "general"}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/overview">
							Overview
						</SidebarNavItem>
					)}
					{permissions.viewAllLicenses && (
						<SidebarNavItem href="/deployment/licenses">
							Licenses
						</SidebarNavItem>
					)}
					{permissions.editDeploymentConfig && (
						<SidebarNavItem href="/deployment/appearance">
							Appearance
						</SidebarNavItem>
					)}
					{permissions.viewAllUsers && (
						<SidebarNavItem href="/deployment/users">Users</SidebarNavItem>
					)}
					{permissions.viewAnyGroup && (
						<SidebarNavItem href="/deployment/groups">
							<div className="flex flex-row items-center gap-1">
								Groups {showOrganizations && <ArrowUpRightIcon size={16} />}
							</div>
						</SidebarNavItem>
					)}
					{permissions.viewNotificationTemplate && (
						<SidebarNavItem href="/deployment/notifications">
							Notifications
						</SidebarNavItem>
					)}
					{!hasPremiumLicense && (
						<SidebarNavItem href="/deployment/premium">Premium</SidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			<SidebarAccordion
				icon={HardDriveIcon}
				label="Infrastructure"
				href="/deployment/security"
				open={openSections.has("infrastructure")}
				onToggle={() => toggleSection("infrastructure")}
				active={activeSection === "infrastructure"}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/security">
							Security
						</SidebarNavItem>
					)}
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/observability">
							Observability
						</SidebarNavItem>
					)}
					{permissions.readWorkspaceProxies && (
						<SidebarNavItem href="/deployment/workspace-proxies">
							Workspace proxies
						</SidebarNavItem>
					)}
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/network">Network</SidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			<SidebarAccordion
				icon={UserLockIcon}
				label="Authentication"
				href="/deployment/userauth"
				open={openSections.has("authentication")}
				onToggle={() => toggleSection("authentication")}
				active={activeSection === "authentication"}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/userauth">
							User authentication
						</SidebarNavItem>
					)}
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/external-auth">
							External authentication
						</SidebarNavItem>
					)}
					{permissions.viewDeploymentConfig &&
						(experiments.includes("oauth2") ||
							getPrereleaseFlag(buildInfo) === "devel") && (
							<SidebarNavItem href="/deployment/oauth2-provider/apps">
								OAuth2 applications
							</SidebarNavItem>
						)}
					{permissions.viewOrganizationIDPSyncSettings && (
						<SidebarNavItem href="/deployment/idp-org-sync">
							IdP organization sync
						</SidebarNavItem>
					)}
				</div>
			</SidebarAccordion>
		</div>
	);
};
