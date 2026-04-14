import { Container, KeyRound, Scale, Settings, Sparkles } from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import type { BuildInfoResponse, Experiment } from "#/api/typesGenerated";
import { SettingsSidebarNavItem as SidebarNavItem } from "#/components/Sidebar/Sidebar";
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
 * sections. Only one accordion is open at a time — navigating to a
 * page auto-opens its section and collapses others.
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
	const [openSection, setOpenSection] =
		useState<DeploymentSection>(activeSection);

	// Sync the open section when the route changes so navigating
	// from outside the sidebar (e.g. breadcrumbs) opens the right
	// accordion.
	useEffect(() => {
		setOpenSection(activeSection);
	}, [activeSection]);

	const toggle = useCallback((section: DeploymentSection) => {
		setOpenSection((prev) => (prev === section ? prev : section));
	}, []);

	return (
		<div className="flex flex-col gap-1">
			{/* General */}
			<SidebarAccordion
				icon={Settings}
				label="General"
				open={openSection === "general"}
				onToggle={() => toggle("general")}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/overview">
							Overview
						</SidebarNavItem>
					)}
					{permissions.editDeploymentConfig && (
						<SidebarNavItem href="/deployment/appearance">
							Appearance
						</SidebarNavItem>
					)}
					{permissions.viewNotificationTemplate && (
						<SidebarNavItem href="/deployment/notifications">
							Notifications
						</SidebarNavItem>
					)}
					{permissions.viewAllUsers && (
						<SidebarNavItem href="/deployment/users">Users</SidebarNavItem>
					)}
					{permissions.viewAllLicenses && (
						<SidebarNavItem href="/deployment/licenses">
							Licenses
						</SidebarNavItem>
					)}
					{!hasPremiumLicense && (
						<SidebarNavItem href="/deployment/premium">Premium</SidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			{/* Infrastructure */}
			<SidebarAccordion
				icon={Container}
				label="Infrastructure"
				open={openSection === "infrastructure"}
				onToggle={() => toggle("infrastructure")}
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
							Workspace Proxies
						</SidebarNavItem>
					)}
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/network">Network</SidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			{/* Authentication */}
			<SidebarAccordion
				icon={KeyRound}
				label="Authentication"
				open={openSection === "authentication"}
				onToggle={() => toggle("authentication")}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/userauth">
							User Authentication
						</SidebarNavItem>
					)}
					{permissions.viewDeploymentConfig &&
						(experiments.includes("oauth2") || isDevBuild(buildInfo)) && (
							<SidebarNavItem href="/deployment/oauth2-provider/apps">
								OAuth2 Applications
							</SidebarNavItem>
						)}
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/external-auth">
							External Authentication
						</SidebarNavItem>
					)}
					{permissions.viewOrganizationIDPSyncSettings && (
						<SidebarNavItem href="/deployment/idp-org-sync">
							Organization Sync
						</SidebarNavItem>
					)}
				</div>
			</SidebarAccordion>

			{/* AI Settings */}
			<SidebarAccordion
				icon={Sparkles}
				label="AI Settings"
				open={openSection === "ai-settings"}
				onToggle={() => toggle("ai-settings")}
			>
				<div className="flex flex-col gap-1">
					<SidebarNavItem href="/deployment/ai-settings/usage-stats">
						Usage Stats
					</SidebarNavItem>
					<SidebarNavItem href="/deployment/ai-settings/models">
						Models
					</SidebarNavItem>
					<SidebarNavItem href="/deployment/ai-settings/providers">
						Providers
					</SidebarNavItem>
					<SidebarNavItem href="/deployment/ai-settings/keys">
						Keys
					</SidebarNavItem>
				</div>
			</SidebarAccordion>

			{/* AI Governance */}
			<SidebarAccordion
				icon={Scale}
				label="AI Governance"
				open={openSection === "ai-governance"}
				onToggle={() => toggle("ai-governance")}
			>
				<div className="flex flex-col gap-1">
					{permissions.viewDeploymentConfig && (
						<SidebarNavItem href="/deployment/ai-governance">
							Access &amp; Speed
						</SidebarNavItem>
					)}
					<SidebarNavItem href="/deployment/ai-governance/settings">
						Settings
					</SidebarNavItem>
					<SidebarNavItem href="/deployment/ai-governance/analytics">
						Analytics
					</SidebarNavItem>
					<SidebarNavItem href="/deployment/ai-governance/data-controls">
						Data Controls
					</SidebarNavItem>
				</div>
			</SidebarAccordion>
		</div>
	);
};
