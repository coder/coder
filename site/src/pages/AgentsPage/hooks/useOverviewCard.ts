import { useEffect, useState } from "react";

/** Keeps the overview card open in every chat and across reloads. */
export const OVERVIEW_CARD_PINNED_KEY = "agents.overview-card-pinned";

/** Open state of the chat overview card and its top bar toggle. */
export type OverviewCardState = {
	isOpen: boolean;
	isPinned: boolean;
	/** Opens the card, or closes and unpins it. */
	toggle: () => void;
	/** Closes the card unless it is pinned. */
	dismiss: () => void;
	setPinned: (pinned: boolean) => void;
};

type OverviewCardRefs = {
	/** Clicks inside the card do not close it. */
	cardRef: React.RefObject<HTMLElement | null>;
	/** Clicks on the toggle do not count as outside, and Escape moves focus here. */
	toggleRef: React.RefObject<HTMLButtonElement | null>;
};

/**
 * An unpinned card closes on an outside click or Escape. The page view
 * mounts again for each chat, so an unpinned card starts closed there.
 */
export const useOverviewCard = ({
	cardRef,
	toggleRef,
}: OverviewCardRefs): OverviewCardState => {
	const [isPinned, setIsPinned] = useState(
		() => localStorage.getItem(OVERVIEW_CARD_PINNED_KEY) === "true",
	);
	const [isOpen, setIsOpen] = useState(isPinned);

	useEffect(() => {
		if (!isOpen || isPinned) {
			return;
		}
		const onPointerDown = (event: PointerEvent) => {
			const target = event.target;
			if (
				target instanceof Node &&
				(cardRef.current?.contains(target) ||
					toggleRef.current?.contains(target))
			) {
				return;
			}
			setIsOpen(false);
		};
		// Escape elsewhere belongs to the composer and its menus.
		const onKeyDown = (event: KeyboardEvent) => {
			const focused = document.activeElement;
			if (
				event.key !== "Escape" ||
				!(
					cardRef.current?.contains(focused) ||
					toggleRef.current?.contains(focused)
				)
			) {
				return;
			}
			// Focus inside the card would drop to the body when it unmounts.
			toggleRef.current?.focus();
			setIsOpen(false);
		};
		document.addEventListener("pointerdown", onPointerDown);
		document.addEventListener("keydown", onKeyDown);
		return () => {
			document.removeEventListener("pointerdown", onPointerDown);
			document.removeEventListener("keydown", onKeyDown);
		};
	}, [isOpen, isPinned, cardRef, toggleRef]);

	const setPinned = (pinned: boolean) => {
		setIsPinned(pinned);
		localStorage.setItem(OVERVIEW_CARD_PINNED_KEY, String(pinned));
	};

	return {
		isOpen,
		isPinned,
		toggle: () => {
			if (isOpen) {
				setPinned(false);
			}
			setIsOpen(!isOpen);
		},
		dismiss: () => {
			if (!isPinned) {
				setIsOpen(false);
			}
		},
		setPinned,
	};
};
