import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem,
} from "#/components/Sidebar/Sidebar";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { canViewWorkspaces } from "#/modules/permissions";
import { getPrereleaseFlag } from "#/utils/buildInfo";

export const Sidebar: React.FC = () => {
	const { entitlements, experiments, buildInfo } = useDashboard();
	const { permissions } = useAuthenticated();
	const showWorkspacePages = canViewWorkspaces(permissions);
	const showSchedulePage =
		entitlements.features.advanced_template_scheduling.enabled &&
		showWorkspacePages;
	const showOAuth2Page =
		experiments.includes("oauth2") || getPrereleaseFlag(buildInfo) === "devel";

	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				<SettingsSidebarNavItem href="account">Account</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="appearance">
					Appearance
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="external-auth">
					External Authentication
				</SettingsSidebarNavItem>
				{showOAuth2Page && (
					<SettingsSidebarNavItem href="oauth2-provider">
						OAuth2 Applications
					</SettingsSidebarNavItem>
				)}
				{showSchedulePage && (
					<SettingsSidebarNavItem href="schedule">
						Schedule
					</SettingsSidebarNavItem>
				)}
				<SettingsSidebarNavItem href="security">
					Security
				</SettingsSidebarNavItem>
				{showWorkspacePages && (
					<SettingsSidebarNavItem href="ssh-keys">
						SSH Keys
					</SettingsSidebarNavItem>
				)}
				<SettingsSidebarNavItem href="tokens">Tokens</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="secrets">Secrets</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="notifications">
					Notifications
				</SettingsSidebarNavItem>
			</div>
		</BaseSidebar>
	);
};
