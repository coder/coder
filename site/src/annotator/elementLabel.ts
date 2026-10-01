// Page-controlled identifiers can be any length; the label is a hint.
const maxLabelLength = 60;

/**
 * A short, human-readable name for an element, shown next to the outline
 * and in the comment popup: `button#save`, `a.nav-link`, `div`.
 */
export function elementLabel(target: Element): string {
	const tag = target.tagName.toLowerCase();
	const testId = target.getAttribute("data-testid");
	const firstClass = Array.from(target.classList).find(
		(name) => !name.includes(":") && !name.includes("["),
	);
	let label = tag;
	if (target.id) {
		label = `${tag}#${target.id}`;
	} else if (testId) {
		label = `${tag}[data-testid=${testId}]`;
	} else if (firstClass) {
		label = `${tag}.${firstClass}`;
	}
	return label.length > maxLabelLength
		? `${label.slice(0, maxLabelLength - 3)}...`
		: label;
}
