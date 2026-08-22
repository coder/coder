// Restore numeric `data-fd-step` on step headings after `rehype-raw`.
//
// Fumadocs numbers steps in the table of contents through its default
// `rehypeToc` plugin, which copies a heading's step index onto the TOC entry
// only when the property is a real number:
//
//   _step: typeof element.properties["data-fd-step"] === "number" ? ... : undefined
//
// `remarkSteps` sets `data-fd-step` as a number, but this site runs `rehype-raw`
// first (to keep the corpus's raw HTML). `rehype-raw` serializes and reparses
// the whole tree, which turns that number into a string and renames the hast
// key to camelCase (`dataFdStep`). By the time `rehypeToc` runs, the numeric
// check fails and the TOC loses its step numbers (the body circles are
// unaffected, since they come from the class-based `.fd-step` CSS counter).
//
// This plugin runs between `rehype-raw` and the default plugins and rewrites
// the value back to a number under the dashed key `rehypeToc` reads.

export type HastNode = {
	type: string;
	tagName?: string;
	properties?: Record<string, unknown>;
	children?: HastNode[];
	[key: string]: unknown;
};

const HEADINGS = new Set(["h1", "h2", "h3", "h4", "h5", "h6"]);

function restoreStepProps(node: HastNode): void {
	if (
		node.type === "element" &&
		node.tagName &&
		HEADINGS.has(node.tagName) &&
		node.properties
	) {
		const props = node.properties;
		const raw = props["data-fd-step"] ?? props.dataFdStep;
		if (raw !== undefined && raw !== null) {
			const value = typeof raw === "number" ? raw : Number(String(raw));
			if (!Number.isNaN(value)) {
				delete props.dataFdStep;
				props["data-fd-step"] = value;
			}
		}
	}
	if (node.children) {
		for (const child of node.children) restoreStepProps(child);
	}
}

export function rehypeStepTocNumbers() {
	return (tree: HastNode): void => {
		restoreStepProps(tree);
	};
}
