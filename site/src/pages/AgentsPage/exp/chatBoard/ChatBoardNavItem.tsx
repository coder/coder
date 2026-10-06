import { LayoutDashboardIcon } from "lucide-react";
import { useLocation } from "react-router";
import { SettingsNavItem } from "../../components/ChatsSidebar/settings/SettingsNavItem";

export const CHAT_BOARD_PATH = "/agents/board";

type ChatBoardNavItemProps = {
	/** Normalized query string the sidebar appends to its own links. */
	readonly locationSearch: string;
};

/** The sidebar's "Board" entry. */
export const ChatBoardNavItem: React.FC<ChatBoardNavItemProps> = ({
	locationSearch,
}) => {
	const location = useLocation();
	return (
		<SettingsNavItem
			icon={LayoutDashboardIcon}
			label="Board"
			active={location.pathname.startsWith(CHAT_BOARD_PATH)}
			to={{ pathname: CHAT_BOARD_PATH, search: locationSearch }}
		/>
	);
};
