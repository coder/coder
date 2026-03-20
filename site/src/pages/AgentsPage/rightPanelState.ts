const RIGHT_PANEL_OPEN_STORAGE_KEY = "agents.right-panel-open";
export const RIGHT_PANEL_WIDTH_STORAGE_KEY = "agents.right-panel-width";
export const RIGHT_PANEL_DEFAULT_WIDTH = 480;
export const RIGHT_PANEL_MIN_WIDTH = 360;
const RIGHT_PANEL_MAX_WIDTH_RATIO = 0.7;

export function getRightPanelMaxWidth(): number {
	if (typeof window === "undefined") {
		return 960;
	}

	return Math.max(
		RIGHT_PANEL_MIN_WIDTH,
		Math.floor(window.innerWidth * RIGHT_PANEL_MAX_WIDTH_RATIO),
	);
}

export function loadPersistedRightPanelWidth(): number {
	if (typeof window === "undefined") {
		return RIGHT_PANEL_DEFAULT_WIDTH;
	}

	const stored = localStorage.getItem(RIGHT_PANEL_WIDTH_STORAGE_KEY);
	if (!stored) {
		return RIGHT_PANEL_DEFAULT_WIDTH;
	}

	const parsed = Number.parseInt(stored, 10);
	if (
		Number.isNaN(parsed) ||
		parsed < RIGHT_PANEL_MIN_WIDTH ||
		parsed > getRightPanelMaxWidth()
	) {
		return RIGHT_PANEL_DEFAULT_WIDTH;
	}

	return parsed;
}

export function getPersistedRightPanelState(): {
	open: boolean;
	width: number;
} {
	if (typeof window === "undefined") {
		return { open: false, width: RIGHT_PANEL_DEFAULT_WIDTH };
	}

	return {
		open: localStorage.getItem(RIGHT_PANEL_OPEN_STORAGE_KEY) === "true",
		width: loadPersistedRightPanelWidth(),
	};
}
