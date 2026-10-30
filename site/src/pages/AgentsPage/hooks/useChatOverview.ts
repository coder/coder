import { useState } from "react";
import type { ChatOverviewToggleProps } from "../components/ChatOverviewCard";
import type { OverviewTarget } from "../components/ChatOverviewCardView";
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
 *
 * A row click unmounts the card, so `recordTarget` numbers each target: the
 * right panel moves focus to its tab once per number, and the Git panel
 * opens the row's view once per number.
 */
export const useChatOverview = ({
	isRightPanelOpen,
}: {
	isRightPanelOpen: boolean;
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const { isEmbedded } = useEmbedContext();
	const [isOpen, setIsOpen] = useState(getPersistedOverviewCardOpen);
	const [request, setRequest] = useState<{
		id: number;
		target: OverviewTarget;
	}>();
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
	return {
		toggle,
		isCardShown: canShow && isOpen,
		recordTarget: (target: OverviewTarget) =>
			setRequest((previous) => ({ id: (previous?.id ?? 0) + 1, target })),
		focusRequestId: request?.id,
		gitViewRequest:
			request?.target.tabId === "git"
				? { id: request.id, view: request.target.view }
				: undefined,
	};
};
