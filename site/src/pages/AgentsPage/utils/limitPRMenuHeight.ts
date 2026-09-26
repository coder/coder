const MAX_VISIBLE_PR_MENU_ITEMS = 5;

/**
 * Callback ref that caps a PR menu at its first five items so longer
 * lists scroll. Rows wrap to different heights, so the cap is measured.
 * The menu needs `relative` and `overflow-y-auto`.
 */
export const limitPRMenuHeight = (menu: HTMLElement | null) => {
	if (!menu) {
		return;
	}
	const items = menu.querySelectorAll<HTMLElement>('[role="menuitem"]');
	if (items.length <= MAX_VISIBLE_PR_MENU_ITEMS) {
		menu.style.removeProperty("max-height");
		return;
	}
	const lastVisible = items[MAX_VISIBLE_PR_MENU_ITEMS - 1];
	const style = getComputedStyle(menu);
	const height =
		lastVisible.offsetTop +
		lastVisible.offsetHeight +
		Number.parseFloat(style.borderTopWidth) +
		Number.parseFloat(style.paddingBottom) +
		Number.parseFloat(style.borderBottomWidth);
	menu.style.maxHeight = `${height}px`;
};
