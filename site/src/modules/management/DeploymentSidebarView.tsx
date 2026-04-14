import { Container, KeyRound, Scale, Settings, Sparkles } from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import type { BuildInfoResponse, Experiment } from "#/api/typesGenerated";
import { SettingsSidebarNavItem as BaseSidebarNavItem } from "#/components/Sidebar/Sidebar";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import type { Permissions } from "#/modules/permissions";
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
	hasPremiumLicense,
	experiments,
	buildInfo,
	activeSection,
}) => {
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

	const toggle = useCallback((section: DeploymentSection) => {
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
			{/* General */}
			<SidebarAccordion
				icon={Settings}
				label="General"
				href="/deployment/overview"
				open={openSections.has("general")}
				onToggle={() => toggle("general")}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/overview">
							Overview
						</BaseSidebarNavItem>
					)}
					{permissions.editDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/appearance">
							Appearance
						</BaseSidebarNavItem>
					)}
					{permissions.viewNotificationTemplate && (
						<BaseSidebarNavItem href="/deployment/notifications">
							Notifications
						</BaseSidebarNavItem>
					)}
					{permissions.viewAllUsers && (
						<BaseSidebarNavItem href="/deployment/users">
							Users
						</BaseSidebarNavItem>
					)}
					{permissions.viewAllLicenses && (
						<BaseSidebarNavItem href="/deployment/licenses">
							Licenses
						</BaseSidebarNavItem>
					)}
					{!hasPremiumLicense && (
						<BaseSidebarNavItem href="/deployment/premium">
							Premium
						</BaseSidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			{/* Infrastructure */}
			<SidebarAccordion
				icon={Container}
				label="Infrastructure"
				href="/deployment/security"
				open={openSections.has("infrastructure")}
				onToggle={() => toggle("infrastructure")}
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
							Workspace Proxies
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
				icon={KeyRound}
				label="Authentication"
				href="/deployment/userauth"
				open={openSections.has("authentication")}
				onToggle={() => toggle("authentication")}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/userauth">
							User Authentication
						</BaseSidebarNavItem>
					)}
					{permissions.viewDeploymentConfig &&
						(experiments.includes("oauth2") || isDevBuild(buildInfo)) && (
							<BaseSidebarNavItem href="/deployment/oauth2-provider/apps">
								OAuth2 Applications
							</BaseSidebarNavItem>
						)}
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/external-auth">
							External Authentication
						</BaseSidebarNavItem>
					)}
					{permissions.viewOrganizationIDPSyncSettings && (
						<BaseSidebarNavItem href="/deployment/idp-org-sync">
							Organization Sync
						</BaseSidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			{/* AI Settings */}
			<SidebarAccordion
				icon={Sparkles}
				label="AI Settings"
				href="/deployment/ai-settings/usage-stats"
				open={openSections.has("ai-settings")}
				onToggle={() => toggle("ai-settings")}
			>
				<div className="flex flex-col gap-1">
					<BaseSidebarNavItem href="/deployment/ai-settings/usage-stats">
						Usage Stats
					</BaseSidebarNavItem>
					<BaseSidebarNavItem href="/deployment/ai-settings/models">
						Models
					</BaseSidebarNavItem>
					<BaseSidebarNavItem href="/deployment/ai-settings/providers">
						Providers
					</BaseSidebarNavItem>
					<BaseSidebarNavItem href="/deployment/ai-settings/keys">
						Keys
					</BaseSidebarNavItem>
				</div>
			</SidebarAccordion>

			{/* AI Governance */}
			<SidebarAccordion
				icon={Scale}
				label="AI Governance"
				href="/deployment/ai-governance"
				open={openSections.has("ai-governance")}
				onToggle={() => toggle("ai-governance")}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<BaseSidebarNavItem href="/deployment/ai-governance">
							Access &amp; Speed
						</BaseSidebarNavItem>
					)}
					<BaseSidebarNavItem href="/deployment/ai-governance/settings">
						Settings
					</BaseSidebarNavItem>
					<BaseSidebarNavItem href="/deployment/ai-governance/analytics">
						Analytics
					</BaseSidebarNavItem>
					<BaseSidebarNavItem href="/deployment/ai-governance/data-controls">
						Data Controls
					</BaseSidebarNavItem>
				</div>
			</SidebarAccordion>
		</div>
	);
};
