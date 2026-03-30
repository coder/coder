import {
	BellIcon,
	BrushIcon,
	CalendarCogIcon,
	FingerprintIcon,
	KeyIcon,
	LockIcon,
	ServerIcon,
	ShieldIcon,
	UserIcon,
} from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { userChatProviderConfigs } from "#/api/queries/chats";
import type { User } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { GitIcon } from "#/components/Icons/GitIcon";
import {
	Sidebar as BaseSidebar,
	SidebarHeader,
	SidebarNavItem,
} from "#/components/Sidebar/Sidebar";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { isDevBuild } from "#/utils/buildInfo";

interface SidebarProps {
	user: User;
}

export const Sidebar: FC<SidebarProps> = ({ user }) => {
	const { entitlements, experiments, buildInfo } = useDashboard();
	const agentsEnabled = experiments.includes("agents") || isDevBuild(buildInfo);
	const { data: providerConfigs } = useQuery({
		...userChatProviderConfigs(),
		enabled: agentsEnabled,
	});
	const showSchedulePage =
		entitlements.features.advanced_template_scheduling.enabled;

	return (
		<BaseSidebar>
			<SidebarHeader
				avatar={<Avatar fallback={user.username} src={user.avatar_url} />}
				title={user.username}
				subtitle={user.email}
			/>
			<SidebarNavItem href="account" icon={UserIcon}>
				Account
			</SidebarNavItem>
			<SidebarNavItem href="appearance" icon={BrushIcon}>
				Appearance
			</SidebarNavItem>
			<SidebarNavItem href="external-auth" icon={GitIcon}>
				External Authentication
			</SidebarNavItem>
			{agentsEnabled && providerConfigs && providerConfigs.length > 0 && (
				<SidebarNavItem href="providers" icon={ServerIcon}>
					Providers
				</SidebarNavItem>
			)}
			{(experiments.includes("oauth2") || isDevBuild(buildInfo)) && (
				<SidebarNavItem href="oauth2-provider" icon={ShieldIcon}>
					OAuth2 Applications
				</SidebarNavItem>
			)}
			{showSchedulePage && (
				<SidebarNavItem href="schedule" icon={CalendarCogIcon}>
					Schedule
				</SidebarNavItem>
			)}
			<SidebarNavItem href="security" icon={LockIcon}>
				Security
			</SidebarNavItem>
			<SidebarNavItem href="ssh-keys" icon={FingerprintIcon}>
				SSH Keys
			</SidebarNavItem>
			<SidebarNavItem href="tokens" icon={KeyIcon}>
				Tokens
			</SidebarNavItem>
			<SidebarNavItem href="notifications" icon={BellIcon}>
				Notifications
			</SidebarNavItem>
		</BaseSidebar>
	);
};
