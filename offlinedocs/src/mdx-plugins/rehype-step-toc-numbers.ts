// Restore numeric `data-fd-step` on step headings after `rehype-raw`.
// rehype-raw reparses the tree, turning the numeric step index into a
// camelCased string; Fumadocs' TOC step numbering needs it back as a number.

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
