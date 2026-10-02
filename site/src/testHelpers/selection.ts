/**
 * Selects the contents of element and returns the text a user would copy.
 * Run it in a real browser (Storybook play functions): the browser
 * serializes the selection from the rendered layout, so block or flex
 * children show up as line breaks. The selection is cleared afterwards so
 * visual snapshots are unaffected.
 */
export const copiedText = (element: Element): string => {
	const selection = window.getSelection();
	if (!selection) {
		throw new Error("The document has no selection.");
	}
	const range = document.createRange();
	range.selectNodeContents(element);
	selection.removeAllRanges();
	selection.addRange(range);
	const text = selection.toString();
	selection.removeAllRanges();
	return text;
};
