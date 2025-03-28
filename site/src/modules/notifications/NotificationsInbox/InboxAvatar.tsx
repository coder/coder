import {
	type InboxNotificationFallbackIcon,
	InboxNotificationFallbackIcons,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	InfoIcon,
	LaptopIcon,
	LayoutTemplateIcon,
	UserIcon,
} from "lucide-react";
import type { FC } from "react";
import type React from "react";

const fallbackIcons: Record<InboxNotificationFallbackIcon, React.ReactNode> = {
	DEFAULT_ICON_WORKSPACE: <LaptopIcon />,
	DEFAULT_ICON_ACCOUNT: <UserIcon />,
	DEFAULT_ICON_TEMPLATE: <LayoutTemplateIcon />,
	DEFAULT_ICON_OTHER: <InfoIcon />,
};

type InboxAvatarProps = {
	icon: string;
};

export const InboxAvatar: FC<InboxAvatarProps> = ({ icon }) => {
	if (isInboxNotificationFallbackIcon(icon)) {
		return <Avatar variant="icon">{fallbackIcons[icon]}</Avatar>;
	}

	return <Avatar variant="icon" src={icon} />;
};

function isInboxNotificationFallbackIcon(
	icon: string,
): icon is InboxNotificationFallbackIcon {
	return (InboxNotificationFallbackIcons as string[]).includes(icon);
}
