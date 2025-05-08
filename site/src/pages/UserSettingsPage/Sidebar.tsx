import type { User } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { GitIcon } from "components/Icons/GitIcon";
import {
	Sidebar as BaseSidebar,
	SidebarHeader,
	SidebarNavItem,
} from "components/Sidebar/Sidebar";
import {
	AccountIcon,
	AppearanceIcon,
	FingerprintOutlinedIcon,
	NotificationsIcon,
	ScheduleIcon,
	SecurityIcon,
	VpnKeyOutlined,
} from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";

interface SidebarProps {
	user: User;
}

export const Sidebar: FC<SidebarProps> = ({ user }) => {
	const { entitlements } = useDashboard();
	const showSchedulePage =
		entitlements.features.advanced_template_scheduling.enabled;

	return (
		<BaseSidebar>
			<SidebarHeader
				avatar={<Avatar fallback={user.username} src={user.avatar_url} />}
				title={user.username}
				subtitle={user.email}
			/>
			<SidebarNavItem href="account" icon={AccountIcon}>
				Account
			</SidebarNavItem>
			<SidebarNavItem href="appearance" icon={AppearanceIcon}>
				Appearance
			</SidebarNavItem>
			<SidebarNavItem href="external-auth" icon={GitIcon}>
				External Authentication
			</SidebarNavItem>
			{showSchedulePage && (
				<SidebarNavItem href="schedule" icon={ScheduleIcon}>
					Schedule
				</SidebarNavItem>
			)}
			<SidebarNavItem href="security" icon={SecurityIcon}>
				Security
			</SidebarNavItem>
			<SidebarNavItem href="ssh-keys" icon={FingerprintOutlinedIcon}>
				SSH Keys
			</SidebarNavItem>
			<SidebarNavItem href="tokens" icon={VpnKeyOutlined}>
				Tokens
			</SidebarNavItem>
			<SidebarNavItem href="notifications" icon={NotificationsIcon}>
				Notifications
			</SidebarNavItem>
		</BaseSidebar>
	);
};
