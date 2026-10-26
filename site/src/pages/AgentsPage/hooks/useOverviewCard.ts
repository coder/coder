import { useState } from "react";

/** Keeps the overview card open in every chat and across reloads. */
export const OVERVIEW_CARD_OPEN_KEY = "agents.overview-card-open";

/** Whether the user left the overview card open. */
export const readOverviewCardOpen = (): boolean =>
	localStorage.getItem(OVERVIEW_CARD_OPEN_KEY) === "true";

/** Open state of the chat overview card and its top bar toggle. */
export type OverviewCardState = {
	isOpen: boolean;
	toggle: () => void;
};

/**
 * Only the top bar toggle opens and closes the card. An open card stays
 * open in every chat and after a reload, until the toggle closes it.
 */
export const useOverviewCard = (): OverviewCardState => {
	const [isOpen, setIsOpen] = useState(readOverviewCardOpen);
	return {
		isOpen,
		toggle: () => {
			localStorage.setItem(OVERVIEW_CARD_OPEN_KEY, String(!isOpen));
			setIsOpen(!isOpen);
		},
	};
};
