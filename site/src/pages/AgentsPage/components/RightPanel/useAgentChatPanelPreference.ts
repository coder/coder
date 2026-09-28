import { useState } from "react";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import { RIGHT_PANEL_OPEN_KEY } from "./RightPanel";

export function useAgentChatPanelPreference(): {
	showSidebarPanel: boolean;
	handleSetShowSidebarPanel: (next: boolean) => void;
} {
	// Right panel open/closed state is owned here so the loading
	// skeleton and the loaded view share the same layout, preventing
	// a horizontal shift when data arrives.
	const [sidebarPanelPreference, setSidebarPanelPreference] = useState(() => {
		return localStorage.getItem(RIGHT_PANEL_OPEN_KEY) === "true";
	});
	// Below the lg breakpoint, chat and the right panel are mutually
	// exclusive, so a panel left open on a wide window would hide chat
	// as soon as the window narrows. Suppression hides the panel while
	// narrow without touching the persisted preference: widening
	// restores the panel, and an explicit toggle overrides it.
	const isBelowLg = useMediaQuery(belowLgViewportMediaQuery);
	const [panelSuppressedOnNarrow, setPanelSuppressedOnNarrow] =
		useState(isBelowLg);
	const [prevIsBelowLg, setPrevIsBelowLg] = useState(isBelowLg);
	// Render-time state adjustment on breakpoint crossings; see
	// https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes
	if (isBelowLg !== prevIsBelowLg) {
		setPrevIsBelowLg(isBelowLg);
		setPanelSuppressedOnNarrow(isBelowLg);
	}
	// Canonical panel visibility: the persisted preference gated by the
	// narrow-viewport suppression. Only this derived value may be
	// rendered or handed to children; the raw preference stays local.
	const showSidebarPanel = sidebarPanelPreference && !panelSuppressedOnNarrow;

	const handleSetShowSidebarPanel = (next: boolean) => {
		setPanelSuppressedOnNarrow(false);
		setSidebarPanelPreference(next);
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, String(next));
	};

	return {
		showSidebarPanel,
		handleSetShowSidebarPanel,
	};
}
