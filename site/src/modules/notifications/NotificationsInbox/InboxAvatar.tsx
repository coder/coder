import { Avatar } from "components/Avatar/Avatar";
import {
	InfoIcon,
	LaptopIcon,
	LayoutTemplateIcon,
	UserIcon,
} from "lucide-react";
import type { FC } from "react";
import type React from "react";

export type InboxIcon =
	| "DEFAULT_WORKSPACE_ICON"
	| "DEFAULT_ACCOUNT_ICON"
	| "DEFAULT_TEMPLATE_ICON"
	| "DEFAULT_OTHER_ICON";

const inboxIcons: Record<InboxIcon, React.ReactNode> = {
	DEFAULT_WORKSPACE_ICON: <LaptopIcon />,
	DEFAULT_ACCOUNT_ICON: <UserIcon />,
	DEFAULT_TEMPLATE_ICON: <LayoutTemplateIcon />,
	DEFAULT_OTHER_ICON: <InfoIcon />,
};

type InboxAvatarProps = {
	icon: InboxIcon;
};

export const InboxAvatar: FC<InboxAvatarProps> = ({ icon }) => {
	return <Avatar variant="icon">{inboxIcons[icon]}</Avatar>;
};
