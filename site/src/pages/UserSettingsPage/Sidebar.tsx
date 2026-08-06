import { LockIcon, SettingsIcon, UserLockIcon } from "lucide-react";
import type { FC } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Sidebar as BaseSidebar,
	SidebarGroup,
	SidebarHeader,
	SidebarNavItem,
} from "#/components/Sidebar";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getPrereleaseFlag } from "#/utils/buildInfo";

export const Sidebar: FC = () => {
	const { user: me } = useAuthenticated();
	const { entitlements, experiments, buildInfo } = useDashboard();
	const showSchedulePage =
		entitlements.features.advanced_template_scheduling.enabled;
	const showOAuth2Page =
		experiments.includes("oauth2") || getPrereleaseFlag(buildInfo) === "devel";

	return (
		<BaseSidebar>
			<SidebarHeader
				avatar={
					<Avatar
						fallback={me.username}
						src={me.avatar_url}
						className="-mx-0.5"
					/>
				}
				title={me.username}
				subtitle={me.email}
			/>
			<div className="flex flex-col gap-1">
				<SidebarGroup
					icon={SettingsIcon}
					label="General"
					href="/settings/account"
				>
					<SidebarNavItem end href="/settings/account">
						Account
					</SidebarNavItem>
					<SidebarNavItem href="/settings/appearance">
						Appearance
					</SidebarNavItem>
					{showSchedulePage && (
						<SidebarNavItem href="/settings/schedule">Schedule</SidebarNavItem>
					)}
					<SidebarNavItem href="/settings/notifications">
						Notifications
					</SidebarNavItem>
				</SidebarGroup>
				<SidebarGroup
					icon={UserLockIcon}
					label="Authentication"
					href="/settings/security"
				>
					<SidebarNavItem end href="/settings/security">
						Password
					</SidebarNavItem>
					<SidebarNavItem href="/settings/external-auth">
						External authentication
					</SidebarNavItem>
					{showOAuth2Page && (
						<SidebarNavItem href="/settings/oauth2-provider">
							OAuth2 applications
						</SidebarNavItem>
					)}
					<SidebarNavItem href="/settings/ssh-keys">SSH keys</SidebarNavItem>
				</SidebarGroup>
				<SidebarGroup icon={LockIcon} label="Security" href="/settings/tokens">
					<SidebarNavItem end href="/settings/tokens">
						Tokens
					</SidebarNavItem>
					<SidebarNavItem href="/settings/secrets">Secrets</SidebarNavItem>
				</SidebarGroup>
			</div>
		</BaseSidebar>
	);
};
