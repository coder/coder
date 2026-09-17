import { LayoutDashboardIcon } from "lucide-react";
import type { FC } from "react";
import { useLocation } from "react-router";
import { SettingsNavItem } from "../../components/ChatsSidebar/settings/SettingsNavItem";
import { CHAT_BOARD_PATH, useChatBoardEnabled } from "./chatBoardFlag";

interface ChatBoardNavItemProps {
	/** Normalized query string the sidebar appends to its own links. */
	readonly locationSearch: string;
}

/** The sidebar's "Board" entry. Renders nothing while the board is off. */
export const ChatBoardNavItem: FC<ChatBoardNavItemProps> = ({
	locationSearch,
}) => {
	const enabled = useChatBoardEnabled();
	const location = useLocation();
	if (!enabled) {
		return null;
	}
	return (
		<SettingsNavItem
			icon={LayoutDashboardIcon}
			label="Board"
			active={location.pathname.startsWith(CHAT_BOARD_PATH)}
			to={{ pathname: CHAT_BOARD_PATH, search: locationSearch }}
		/>
	);
};
