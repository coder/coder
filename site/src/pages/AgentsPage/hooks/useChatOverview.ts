import { useState } from "react";
import type { ChatOverviewToggleProps } from "../components/ChatOverviewCard";
import { useEmbedContext } from "../components/EmbedContext";
import {
	getPersistedOverviewCardOpen,
	savePersistedOverviewCardOpen,
} from "../utils/overviewCardStorage";
import { useChatFullWidth } from "./useChatFullWidth";

/**
 * The overview card's state on the chat page. The card and its toggle need
 * the centered transcript and a closed right panel, so `toggle` is absent
 * otherwise. The open choice is stored for every chat.
 */
export const useChatOverview = ({
	isRightPanelOpen,
}: {
	isRightPanelOpen: boolean;
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const { isEmbedded } = useEmbedContext();
	const [isOpen, setIsOpen] = useState(getPersistedOverviewCardOpen);
	const canShow = !isRightPanelOpen && !chatFullWidth && !isEmbedded;
	const toggle: ChatOverviewToggleProps | undefined = canShow
		? {
				isOpen,
				onToggle: () => {
					savePersistedOverviewCardOpen(!isOpen);
					setIsOpen(!isOpen);
				},
			}
		: undefined;
	return { toggle, isCardShown: canShow && isOpen };
};
