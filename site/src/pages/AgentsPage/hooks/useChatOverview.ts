import { useEffect, useState } from "react";
import { useEmbedContext } from "../components/EmbedContext";
import {
	getPersistedOverviewCardOpen,
	savePersistedOverviewCardOpen,
} from "../utils/overviewCardStorage";
import { useChatFullWidth } from "./useChatFullWidth";

/**
 * The narrowest chat that fits the 16rem card beside the centered 48rem
 * transcript, with a 1rem inset and a 1rem gap on each side.
 */
const OVERVIEW_MIN_CHAT_WIDTH_REM = 84;

/**
 * The overview card's state on the chat page. The card and its toggle need
 * a chat at least 84rem wide, a centered, non-embedded transcript and a
 * closed right panel, so `toggle` is absent otherwise. Pass `observeChat`
 * as the ref of the chat's outer element, which the hook measures. The open
 * choice is stored for every chat.
 */
export const useChatOverview = ({
	isRightPanelOpen,
}: {
	isRightPanelOpen: boolean;
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const { isEmbedded } = useEmbedContext();
	const [isOpen, setIsOpen] = useState(getPersistedOverviewCardOpen);
	const [chatElement, setChatElement] = useState<HTMLElement | null>(null);
	const [isChatWide, setIsChatWide] = useState(false);
	useEffect(() => {
		if (!chatElement) {
			return;
		}
		const observer = new ResizeObserver(([entry]) => {
			const remPx = Number.parseFloat(
				getComputedStyle(document.documentElement).fontSize,
			);
			setIsChatWide(
				entry.contentRect.width >= OVERVIEW_MIN_CHAT_WIDTH_REM * remPx,
			);
		});
		observer.observe(chatElement);
		return () => observer.disconnect();
	}, [chatElement]);
	const canShow =
		isChatWide && !isRightPanelOpen && !chatFullWidth && !isEmbedded;
	return {
		observeChat: setChatElement,
		toggle: canShow
			? {
					isOpen,
					onToggle: () => {
						savePersistedOverviewCardOpen(!isOpen);
						setIsOpen(!isOpen);
					},
				}
			: undefined,
		isCardShown: canShow && isOpen,
	};
};
