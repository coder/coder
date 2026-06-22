export const LEFT_SIDEBAR_STORAGE_KEY = "agents.left-sidebar-width";
export const LEFT_SIDEBAR_MIN_WIDTH = 240;
export const LEFT_SIDEBAR_DEFAULT_WIDTH = 320;
export const AGENTS_MAIN_PANEL_MIN_WIDTH = 360;
// One rem gives keyboard users a predictable, fine-grained step.
export const LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP = 16;
const LEFT_SIDEBAR_MAX_WIDTH = 660;
const LEFT_SIDEBAR_MAX_WIDTH_RATIO = 0.7;

export function getLeftSidebarMaxWidth(): number {
	const maxWidthWithMainPanel = Math.max(
		LEFT_SIDEBAR_MIN_WIDTH,
		innerWidth - AGENTS_MAIN_PANEL_MIN_WIDTH,
	);

	return Math.max(
		LEFT_SIDEBAR_MIN_WIDTH,
		Math.min(
			LEFT_SIDEBAR_MAX_WIDTH,
			Math.floor(innerWidth * LEFT_SIDEBAR_MAX_WIDTH_RATIO),
			maxWidthWithMainPanel,
		),
	);
}

export function clampLeftSidebarWidth(width: number): number {
	if (!Number.isFinite(width)) {
		return clampLeftSidebarWidth(LEFT_SIDEBAR_DEFAULT_WIDTH);
	}
	return Math.min(
		getLeftSidebarMaxWidth(),
		Math.max(LEFT_SIDEBAR_MIN_WIDTH, Math.round(width)),
	);
}

export function loadPersistedLeftSidebarWidth(): number {
	let stored: string | null;
	try {
		stored = localStorage.getItem(LEFT_SIDEBAR_STORAGE_KEY);
	} catch {
		return clampLeftSidebarWidth(LEFT_SIDEBAR_DEFAULT_WIDTH);
	}

	if (!stored) {
		return clampLeftSidebarWidth(LEFT_SIDEBAR_DEFAULT_WIDTH);
	}

	const parsed = Number.parseInt(stored, 10);
	if (
		Number.isNaN(parsed) ||
		parsed < LEFT_SIDEBAR_MIN_WIDTH ||
		parsed > LEFT_SIDEBAR_MAX_WIDTH
	) {
		return clampLeftSidebarWidth(LEFT_SIDEBAR_DEFAULT_WIDTH);
	}

	return clampLeftSidebarWidth(parsed);
}

export function persistLeftSidebarWidth(width: number): void {
	try {
		localStorage.setItem(LEFT_SIDEBAR_STORAGE_KEY, String(width));
	} catch {
		// Ignore storage failures because resizing still works for this session.
	}
}
