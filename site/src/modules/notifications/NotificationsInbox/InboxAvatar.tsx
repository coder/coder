import {
	InboxNotificationFallbackIconAccount,
	InboxNotificationFallbackIconOther,
	InboxNotificationFallbackIconTemplate,
	InboxNotificationFallbackIconWorkspace,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	InfoIcon,
	LaptopIcon,
	LayoutTemplateIcon,
	RocketIcon,
	UserIcon,
} from "lucide-react";
import type React from "react";
import type { FC } from "react";

const CHANGELOG_ICON = "DEFAULT_ICON_CHANGELOG";

const InboxNotificationFallbackIcons = [
	InboxNotificationFallbackIconAccount,
	InboxNotificationFallbackIconWorkspace,
	InboxNotificationFallbackIconTemplate,
	CHANGELOG_ICON,
	InboxNotificationFallbackIconOther,
] as const;

type InboxNotificationFallbackIcon =
	(typeof InboxNotificationFallbackIcons)[number];

const fallbackIcons: Record<InboxNotificationFallbackIcon, React.ReactNode> = {
	DEFAULT_ICON_WORKSPACE: <LaptopIcon />,
	DEFAULT_ICON_ACCOUNT: <UserIcon />,
	DEFAULT_ICON_TEMPLATE: <LayoutTemplateIcon />,
	DEFAULT_ICON_CHANGELOG: <RocketIcon />,
	DEFAULT_ICON_OTHER: <InfoIcon />,
};

type InboxAvatarProps = {
	icon: string;
};

export const InboxAvatar: FC<InboxAvatarProps> = ({ icon }) => {
	if (icon === "") {
		return <Avatar variant="icon">{fallbackIcons.DEFAULT_ICON_OTHER}</Avatar>;
	}

	if (isInboxNotificationFallbackIcon(icon)) {
		return <Avatar variant="icon">{fallbackIcons[icon]}</Avatar>;
	}

	return <Avatar variant="icon" src={icon} />;
};

function isInboxNotificationFallbackIcon(
	icon: string,
): icon is InboxNotificationFallbackIcon {
	return (InboxNotificationFallbackIcons as readonly string[]).includes(icon);
}
