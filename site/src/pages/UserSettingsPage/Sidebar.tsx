import type { FC } from "react";
import type { User } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { FeatureStageBadge } from "#/components/FeatureStageBadge/FeatureStageBadge";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem,
	SidebarHeader,
} from "#/components/Sidebar/Sidebar";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getPrereleaseFlag } from "#/utils/buildInfo";

interface SidebarProps {
	user: User;
}

export const Sidebar: FC<SidebarProps> = ({ user }) => {
	const { entitlements, experiments, buildInfo } = useDashboard();
	const showSchedulePage =
		entitlements.features.advanced_template_scheduling.enabled;
	const showOAuth2Page =
		experiments.includes("oauth2") || getPrereleaseFlag(buildInfo) === "devel";

	return (
		<BaseSidebar>
			<SidebarHeader
				avatar={<Avatar fallback={user.username} src={user.avatar_url} />}
				title={user.username}
				subtitle={user.email}
			/>
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
				<SettingsSidebarNavItem href="ssh-keys">
					SSH Keys
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="tokens">Tokens</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="secrets">
					<span className="flex min-w-0 items-center gap-2">
						<span>Secrets</span>
						<FeatureStageBadge
							aria-hidden="true"
							contentType="beta"
							size="sm"
						/>
					</span>
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="notifications">
					Notifications
				</SettingsSidebarNavItem>
			</div>
		</BaseSidebar>
	);
};
