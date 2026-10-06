const arrowKeys = new Set(["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"]);
const maxRenderFrames = 30;

const isEmoji = (el: unknown): el is HTMLButtonElement =>
	el instanceof HTMLButtonElement && el.matches(".category button");

/**
 * Makes emojis in an emoji-mart picker keyboard focusable. The library
 * renders emojis with tabindex="-1" and only moves a highlight while focus
 * stays in the search input. Arrow keys pressed on a focused emoji are
 * replayed on the search input, so the library keeps handling the grid
 * layout, search results, and virtualized rows, and focus follows the emoji
 * it highlights. Returns a cleanup function.
 */
export const enableEmojiGridNavigation = (root: ShadowRoot): (() => void) => {
	const search = () =>
		root.querySelector<HTMLInputElement>("input[type=search]");
	const highlighted = () =>
		root.querySelector<HTMLButtonElement>(".category button[aria-selected]");

	// emoji-mart re-renders asynchronously, so retry over a few frames.
	const focusWhenRendered = (
		find: () => HTMLElement | null,
		beforeFrame?: () => void,
	) => {
		let frames = 0;
		const focus = () => {
			const el = find();
			if (el) {
				el.focus();
			} else if (frames++ < maxRenderFrames) {
				beforeFrame?.();
				requestAnimationFrame(focus);
			}
		};
		requestAnimationFrame(focus);
	};

	const moveHighlight = (key: string, from: Element | null) => {
		search()?.dispatchEvent(new KeyboardEvent("keydown", { key }));
		focusWhenRendered(() => {
			const emoji = highlighted();
			return emoji !== from ? emoji : null;
		});
	};

	const handleKeyDown = (event: Event) => {
		const { key, shiftKey, target } = event as KeyboardEvent;
		if (!isEmoji(target)) {
			return;
		}
		if (arrowKeys.has(key)) {
			event.preventDefault();
			moveHighlight(key, target);
		} else if (key === "Tab" && !shiftKey) {
			// The skin tone button is replaced by the emoji preview while an
			// emoji is highlighted. Clear the highlight the way the library
			// does on mouseleave (ignored briefly after scrolling, hence the
			// retries), then move focus to the button once it renders.
			event.preventDefault();
			const clear = () => target.dispatchEvent(new MouseEvent("mouseleave"));
			clear();
			focusWhenRendered(
				() => root.querySelector<HTMLElement>(".skin-tone-button"),
				clear,
			);
		}
	};

	// Tabbing lands on the grid's scroll container. Move focus onto an
	// emoji, or back to search when Shift+Tab leaves an emoji.
	const handleFocusIn = (event: Event) => {
		const { target, relatedTarget } = event as FocusEvent;
		if (
			!(target instanceof HTMLElement) ||
			!target.matches(".scroll:focus-visible")
		) {
			return;
		}
		if (isEmoji(relatedTarget)) {
			search()?.focus();
			return;
		}
		const emoji = highlighted();
		if (emoji) {
			emoji.focus();
		} else {
			moveHighlight("ArrowDown", null);
		}
	};

	// Browsers only make scrollers tabbable while they overflow, and short
	// search results do not, so give the grid a permanent tab stop once
	// emoji-mart, which renders asynchronously, has created it.
	const addTabStop = () => {
		const scroll = root.querySelector(".scroll");
		scroll?.setAttribute("tabindex", "0");
		return scroll !== null;
	};
	const observer = new MutationObserver(() => {
		if (addTabStop()) {
			observer.disconnect();
		}
	});
	if (!addTabStop()) {
		observer.observe(root, { childList: true, subtree: true });
	}
	root.addEventListener("keydown", handleKeyDown);
	root.addEventListener("focusin", handleFocusIn);
	return () => {
		observer.disconnect();
		root.removeEventListener("keydown", handleKeyDown);
		root.removeEventListener("focusin", handleFocusIn);
	};
};
