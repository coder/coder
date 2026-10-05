/**
 * Keyboard navigation for the emoji-mart grid.
 *
 * emoji-mart renders every emoji button with tabindex="-1" and only
 * highlights emojis while DOM focus stays in the search input, so keyboard
 * and screen reader users cannot focus an individual emoji. This moves real
 * focus into the grid:
 *
 * - Tabbing onto the grid's scroll container focuses an emoji instead (the
 *   last focused one, or the first visible one). Shift+Tab from an emoji
 *   returns to the search input.
 * - Arrow keys move focus between emojis. Home and End jump to the first and
 *   last emoji of the current row.
 * - Enter and Space activate the focused emoji through the native button.
 *
 * Category rows are virtualized, so moving into a row that has not rendered
 * yet scrolls it into view and waits for its buttons to appear.
 */

const scrollSelector = ".scroll";
const emojiSelector = ".category button";
// Category rows, plus the rows emoji-mart renders for search results.
const rowSelector = ".category .row, .category > div > .flex";
// emoji-mart hides the category list with an inline style while searching.
const hiddenSelector = '[style*="display: none"]';
const maxRenderFrames = 30;

const isVisible = (el: Element) => el.closest(hiddenSelector) === null;

const getRows = (root: ShadowRoot): HTMLElement[] =>
	Array.from(root.querySelectorAll<HTMLElement>(rowSelector)).filter(isVisible);

const getButtons = (row: Element): HTMLButtonElement[] =>
	Array.from(row.children).filter(
		(child): child is HTMLButtonElement => child instanceof HTMLButtonElement,
	);

const isEmojiButton = (el: Element | null): el is HTMLButtonElement =>
	el instanceof HTMLButtonElement && el.matches(emojiSelector);

const nextFrame = () =>
	new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

/**
 * Resolves the buttons of a row, scrolling it into view and waiting for
 * emoji-mart to render it when it is virtualized away.
 */
const renderRow = async (row: HTMLElement): Promise<HTMLButtonElement[]> => {
	let buttons = getButtons(row);
	if (buttons.length > 0) {
		return buttons;
	}
	row.scrollIntoView({ block: "nearest" });
	for (let i = 0; i < maxRenderFrames && buttons.length === 0; i++) {
		await nextFrame();
		buttons = getButtons(row);
	}
	return buttons;
};

const focusEmoji = (button: HTMLButtonElement) => {
	button.focus();
	button.scrollIntoView({ block: "nearest" });
};

const firstVisibleEmoji = (
	root: ShadowRoot,
	scroll: HTMLElement,
): HTMLButtonElement | undefined => {
	const top = scroll.getBoundingClientRect().top;
	let first: HTMLButtonElement | undefined;
	for (const row of getRows(root)) {
		const button = getButtons(row)[0];
		if (!button) {
			continue;
		}
		first ??= button;
		if (button.getBoundingClientRect().bottom > top) {
			return button;
		}
	}
	return first;
};

const moveFocus = async (
	root: ShadowRoot,
	current: HTMLButtonElement,
	key: string,
) => {
	const rows = getRows(root);
	const rowIndex = rows.findIndex((row) => row.contains(current));
	if (rowIndex === -1) {
		return;
	}
	const buttons = getButtons(rows[rowIndex]);
	const column = buttons.indexOf(current);

	switch (key) {
		case "ArrowLeft":
		case "ArrowRight": {
			const step = key === "ArrowLeft" ? -1 : 1;
			const sibling = buttons[column + step];
			if (sibling) {
				focusEmoji(sibling);
				return;
			}
			const row = rows[rowIndex + step];
			if (!row) {
				return;
			}
			const rowButtons = await renderRow(row);
			const target = step < 0 ? rowButtons.at(-1) : rowButtons[0];
			if (target) {
				focusEmoji(target);
			}
			return;
		}
		case "ArrowUp":
		case "ArrowDown": {
			const row = rows[rowIndex + (key === "ArrowUp" ? -1 : 1)];
			if (!row) {
				return;
			}
			const rowButtons = await renderRow(row);
			// Short trailing rows clamp to their last emoji.
			const target = rowButtons[Math.min(column, rowButtons.length - 1)];
			if (target) {
				focusEmoji(target);
			}
			return;
		}
		case "Home":
			focusEmoji(buttons[0]);
			return;
		case "End":
			focusEmoji(buttons[buttons.length - 1]);
			return;
	}
};

const navigationKeys = new Set([
	"ArrowLeft",
	"ArrowRight",
	"ArrowUp",
	"ArrowDown",
	"Home",
	"End",
]);

/**
 * Enables keyboard focus and arrow key navigation for the emojis in an
 * emoji-mart picker. Returns a cleanup function that removes the listeners.
 */
export const enableEmojiGridNavigation = (root: ShadowRoot): (() => void) => {
	let lastFocused: HTMLButtonElement | undefined;

	const handleFocusIn = (event: Event) => {
		const target = event.target as Element;
		if (isEmojiButton(target)) {
			lastFocused = target;
			return;
		}
		if (!(target instanceof HTMLElement) || !target.matches(scrollSelector)) {
			return;
		}
		// Only redirect keyboard focus; a pointer press on the scroller
		// should not jump to an emoji.
		if (!target.matches(":focus-visible")) {
			return;
		}
		const from = (event as FocusEvent).relatedTarget as Element | null;
		if (isEmojiButton(from)) {
			// Shift+Tab out of the grid lands on its scroll container first.
			root.querySelector<HTMLInputElement>("input[type=search]")?.focus();
			return;
		}
		const emoji =
			lastFocused?.isConnected && isVisible(lastFocused)
				? lastFocused
				: firstVisibleEmoji(root, target);
		emoji?.focus();
	};

	const handleKeyDown = (event: Event) => {
		const keyEvent = event as KeyboardEvent;
		const target = keyEvent.target as Element;
		if (
			!isEmojiButton(target) ||
			!navigationKeys.has(keyEvent.key) ||
			keyEvent.altKey ||
			keyEvent.ctrlKey ||
			keyEvent.metaKey
		) {
			return;
		}
		keyEvent.preventDefault();
		void moveFocus(root, target, keyEvent.key);
	};

	// Chrome and Firefox make scrollers focusable on their own, Safari does
	// not. Without a tab stop the grid would be unreachable there. The picker
	// renders asynchronously, so wait for the scroller to exist.
	const ensureScrollTabStop = () => {
		const scroll = root.querySelector<HTMLElement>(scrollSelector);
		if (!scroll) {
			return false;
		}
		if (!scroll.hasAttribute("tabindex")) {
			scroll.tabIndex = 0;
		}
		return true;
	};
	const observer = new MutationObserver(() => {
		if (ensureScrollTabStop()) {
			observer.disconnect();
		}
	});
	if (!ensureScrollTabStop()) {
		observer.observe(root, { childList: true, subtree: true });
	}

	root.addEventListener("focusin", handleFocusIn);
	root.addEventListener("keydown", handleKeyDown);
	return () => {
		observer.disconnect();
		root.removeEventListener("focusin", handleFocusIn);
		root.removeEventListener("keydown", handleKeyDown);
	};
};
