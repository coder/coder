import { useEffect, useId, useState } from "react";
import type { OverviewTarget } from "../components/ChatOverviewCardView";
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
 * choice is stored for every chat. Give the card `cardId`, which the toggle
 * names in `aria-controls`.
 *
 * A row opens the right panel and unmounts the focused row, so
 * `focusRequestId` asks the panel to focus its active tab once per
 * `recordTarget` call.
 */
export const useChatOverview = ({
	isRightPanelOpen,
}: {
	isRightPanelOpen: boolean;
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const { isEmbedded } = useEmbedContext();
	const cardId = useId();
	const [isOpen, setIsOpen] = useState(getPersistedOverviewCardOpen);
	const [request, setRequest] = useState<{
		id: number;
		target: OverviewTarget;
	}>();
	const [appliedRequestIds, setAppliedRequestIds] = useState<{
		focus?: number;
	}>({});
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
		cardId,
		toggle: canShow
			? {
					isOpen,
					cardId,
					onToggle: () => {
						savePersistedOverviewCardOpen(!isOpen);
						setIsOpen(!isOpen);
					},
				}
			: undefined,
		isCardShown: canShow && isOpen,
		recordTarget: (target: OverviewTarget) =>
			setRequest((previous) => ({ id: (previous?.id ?? 0) + 1, target })),
		focusRequestId:
			request && request.id !== appliedRequestIds.focus
				? request.id
				: undefined,
		onFocusRequestApplied: (id: number) =>
			setAppliedRequestIds((ids) => ({ ...ids, focus: id })),
	};
};
