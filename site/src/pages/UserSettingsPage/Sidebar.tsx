import { Sidebar as BaseSidebar, SidebarNavItem } from "#/components/Sidebar";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getPrereleaseFlag } from "#/utils/buildInfo";

export const Sidebar: React.FC = () => {
	const { entitlements, experiments, buildInfo } = useDashboard();
	const showSchedulePage =
		entitlements.features.advanced_template_scheduling.enabled;
	const showOAuth2Page =
		experiments.includes("oauth2") || getPrereleaseFlag(buildInfo) === "devel";

	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				<SidebarNavItem href="account">Account</SidebarNavItem>
				<SidebarNavItem href="appearance">Appearance</SidebarNavItem>
				<SidebarNavItem href="external-auth">
					External Authentication
				</SidebarNavItem>
				{showOAuth2Page && (
					<SidebarNavItem href="oauth2-provider">
						OAuth2 Applications
					</SidebarNavItem>
				)}
				{showSchedulePage && (
					<SidebarNavItem href="schedule">Schedule</SidebarNavItem>
				)}
				<SidebarNavItem href="security">Security</SidebarNavItem>
				<SidebarNavItem href="ssh-keys">SSH Keys</SidebarNavItem>
				<SidebarNavItem href="tokens">Tokens</SidebarNavItem>
				<SidebarNavItem href="secrets">Secrets</SidebarNavItem>
				<SidebarNavItem href="notifications">Notifications</SidebarNavItem>
			</div>
		</BaseSidebar>
	);
};
