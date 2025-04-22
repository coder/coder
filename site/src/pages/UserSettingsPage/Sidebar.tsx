import AppearanceIcon from "@mui/icons-material/Brush";
import ScheduleIcon from "@mui/icons-material/EditCalendarOutlined";
import FingerprintOutlinedIcon from "@mui/icons-material/FingerprintOutlined";
import SecurityIcon from "@mui/icons-material/LockOutlined";
import NotificationsIcon from "@mui/icons-material/NotificationsNoneOutlined";
import AccountIcon from "@mui/icons-material/Person";
import VpnKeyOutlined from "@mui/icons-material/VpnKeyOutlined";
import type { User } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { GitIcon } from "components/Icons/GitIcon";
import {
	Sidebar as BaseSidebar,
	SidebarHeader,
	SidebarNavItem,
} from "components/Sidebar/Sidebar";
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
