/**
 * Focuses the first of the elements that the browser renders, since a
 * control that CSS hides on a small screen cannot take focus. Returns whether
 * one took it.
 */
export const focusFirstVisible = (
	elements: readonly (HTMLElement | null)[],
): boolean => {
	const target = elements.find(
		(element) => element?.isConnected && (element.checkVisibility?.() ?? true),
	);
	target?.focus();
	return target !== undefined;
};
