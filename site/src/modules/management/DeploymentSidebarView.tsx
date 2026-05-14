import {
	HardDrive,
	PanelLeft,
	Settings,
	UserLock,
} from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import type { BuildInfoResponse, Experiment } from "#/api/typesGenerated";
import { SettingsSidebarNavItem as BaseSidebarNavItem } from "#/components/Sidebar/Sidebar";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import type { Permissions } from "#/modules/permissions";
import { cn } from "#/utils/cn";
import { isDevBuild } from "#/utils/buildInfo";
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
 * sections. Section headers toggle freely — multiple sections can be
 * open at once. Clicking a sub-item link collapses all other sections
 * so only the active section stays open after navigation.
 */
export const DeploymentSidebarView: FC<DeploymentSidebarViewProps> = ({
	permissions,
	// showOrganizations is passed through for future use by the
	// Groups link (external redirect indicator).
	showOrganizations: _showOrganizations,
	hasPremiumLicense: _hasPremiumLicense,
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
					<span className="text-sm text-content-secondary">
						Deployment
					</span>
				)}
				<PanelLeft className={cn(
					"size-4 text-content-secondary group-hover:text-content-primary transition-colors",
					!collapsed && "ml-auto",
				)} />
			</button>
			{/* General */}
			<SidebarAccordion
				icon={Settings}
				label="General"
				href="/deployment/overview"
				open={openSections.has("general")}
				onToggle={() => toggleSection("general")}
				active={activeSection === "general"}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/overview">
							Overview
						</BaseSidebarNavItem>
					)}
					{permissions.viewAllLicenses && (
						<BaseSidebarNavItem href="/deployment/licenses">
							Licenses
						</BaseSidebarNavItem>
					)}
					{permissions.editDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/appearance">
							Appearance
						</BaseSidebarNavItem>
					)}
					{permissions.viewAllUsers && (
						<BaseSidebarNavItem href="/deployment/users">
							Users
						</BaseSidebarNavItem>
					)}
					<BaseSidebarNavItem href="/deployment/secrets">
						Secrets
					</BaseSidebarNavItem>
				</div>
			</SidebarAccordion>

			{/* Infrastructure */}
			<SidebarAccordion
				icon={HardDrive}
				label="Infrastructure"
				href="/deployment/security"
				open={openSections.has("infrastructure")}
				onToggle={() => toggleSection("infrastructure")}
				active={activeSection === "infrastructure"}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/security">
							Security
						</BaseSidebarNavItem>
					)}
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/observability">
							Observability
						</BaseSidebarNavItem>
					)}
					{permissions.readWorkspaceProxies && (
						<BaseSidebarNavItem href="/deployment/workspace-proxies">
							Workspace proxies
						</BaseSidebarNavItem>
					)}
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/network">
							Network
						</BaseSidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			{/* Authentication */}
			<SidebarAccordion
				icon={UserLock}
				label="Authentication"
				href="/deployment/userauth"
				open={openSections.has("authentication")}
				onToggle={() => toggleSection("authentication")}
				active={activeSection === "authentication"}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/userauth">
							User authentication
						</BaseSidebarNavItem>
					)}
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/external-auth">
							External authentication
						</BaseSidebarNavItem>
					)}
					{permissions.viewDeploymentConfig &&
						(experiments.includes("oauth2") || isDevBuild(buildInfo)) && (
							<BaseSidebarNavItem href="/deployment/oauth2-provider/apps">
								OAuth2 applications
							</BaseSidebarNavItem>
						)}
					{permissions.viewOrganizationIDPSyncSettings && (
						<BaseSidebarNavItem href="/deployment/idp-org-sync">
							IdP organization sync
						</BaseSidebarNavItem>
					)}
				</div>
			</SidebarAccordion>
		</div>
	);
};
