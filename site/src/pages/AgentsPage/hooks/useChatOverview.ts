import { useState } from "react";
import type { OverviewTarget } from "../components/ChatOverviewCardView";
import { useEmbedContext } from "../components/EmbedContext";
import {
	getPersistedOverviewCardOpen,
	savePersistedOverviewCardOpen,
} from "../utils/overviewCardStorage";
import { useChatFullWidth } from "./useChatFullWidth";

/**
 * The overview card's state on the chat page. The card and its toggle need
 * a centered, non-embedded transcript and a closed right panel, so `toggle`
 * is absent otherwise. The open choice is stored for every chat.
 *
 * `recordTarget` numbers each click, so repeating one target still moves
 * focus and opens its Git view.
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
	const gitViewRequest =
		request?.target.tabId === "git"
			? { id: request.id, view: request.target.view }
			: undefined;
	return {
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
		recordTarget: (target: OverviewTarget) =>
			setRequest((previous) => ({ id: (previous?.id ?? 0) + 1, target })),
		focusRequestId: request?.id,
		gitViewRequest,
	};
};
