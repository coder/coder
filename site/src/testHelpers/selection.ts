/**
 * Returns the text a user would copy from element. Needs a real browser
 * (Storybook play functions).
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
