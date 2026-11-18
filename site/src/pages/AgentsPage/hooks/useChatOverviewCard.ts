import { useId, useLayoutEffect, useState } from "react";
import type { ChatOverviewToggleProps } from "../components/ChatOverviewCard";
import { useEmbedContext } from "../components/EmbedContext";
import {
	getPersistedOverviewCardOpen,
	savePersistedOverviewCardOpen,
} from "../utils/overviewCardStorage";
import { useChatFullWidth } from "./useChatFullWidth";

const OVERVIEW_MIN_CHAT_WIDTH_REM = 72;
const OVERVIEW_CENTERED_MIN_CHAT_WIDTH_REM = 84;

type ChatOverviewCardState = {
	/** Attach to the element whose width decides whether the card fits. */
	chatRef: (element: HTMLElement | null) => void;
	cardId: string;
	/** Absent while the chat cannot show the card. */
	toggle: ChatOverviewToggleProps | undefined;
	isCardShown: boolean;
	/**
	 * Sets `--chat-overview-reserve`, the right padding the transcript and
	 * composer leave so the card does not cover them.
	 */
	reserveClassName: string | undefined;
};

/**
 * Decides whether the chat shows the overview card.
 */
export function useChatOverviewCard(
	showSidebarPanel: boolean,
): ChatOverviewCardState {
	const [chatFullWidth] = useChatFullWidth();
	const { isEmbedded } = useEmbedContext();
	const cardId = useId();
	const [isOpen, setIsOpen] = useState(getPersistedOverviewCardOpen);
	const [chatElement, setChatElement] = useState<HTMLElement | null>(null);
	const [chatFit, setChatFit] = useState({
		showsOverview: false,
		fitsCentered: false,
	});
	useLayoutEffect(() => {
		if (!chatElement) {
			return;
		}
		const updateFit = (width: number) => {
			const remPx = Number.parseFloat(
				getComputedStyle(document.documentElement).fontSize,
			);
			const showsOverview = width >= OVERVIEW_MIN_CHAT_WIDTH_REM * remPx;
			const fitsCentered =
				width >= OVERVIEW_CENTERED_MIN_CHAT_WIDTH_REM * remPx;
			setChatFit((current) =>
				current.showsOverview === showsOverview &&
				current.fitsCentered === fitsCentered
					? current
					: { showsOverview, fitsCentered },
			);
		};
		updateFit(chatElement.getBoundingClientRect().width);
		const observer = new ResizeObserver(([entry]) => {
			updateFit(entry.contentRect.width);
		});
		observer.observe(chatElement);
		return () => observer.disconnect();
	}, [chatElement]);

	const canShowOverview =
		chatFit.showsOverview && !showSidebarPanel && !chatFullWidth && !isEmbedded;
	const isCardShown = canShowOverview && isOpen;
	return {
		chatRef: setChatElement,
		cardId,
		toggle: canShowOverview
			? {
					isOpen,
					cardId,
					onToggle: () => {
						savePersistedOverviewCardOpen(!isOpen);
						setIsOpen(!isOpen);
					},
				}
			: undefined,
		isCardShown,
		reserveClassName:
			isCardShown && !chatFit.fitsCentered
				? "[--chat-overview-reserve:18rem]"
				: undefined,
	};
}
