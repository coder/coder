/**
 * Focuses the first of the elements that the browser renders, since a
 * control that CSS hides on a small screen cannot take focus. Returns whether
 * one took it.
 */
export const focusFirstVisible = (
	elements: readonly (HTMLElement | null)[],
): boolean => {
	const target = elements.find(
		(element) => element?.isConnected && isRendered(element),
	);
	target?.focus();
	return target !== undefined;
};

// Safari before 17.4 lacks checkVisibility, so walk the ancestors instead.
const isRendered = (element: HTMLElement): boolean => {
	if (element.checkVisibility) {
		return element.checkVisibility();
	}
	for (let node: Element | null = element; node; node = node.parentElement) {
		if (getComputedStyle(node).display === "none") {
			return false;
		}
	}
	return true;
};
