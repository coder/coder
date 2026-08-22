// Shared mdast helpers for all remark plugins in this directory
// (remark-coder-tabs, remark-details-accordion, remark-github-callouts). All
// walk or inspect a node tree, so the minimal working node shape and the `html`
// node type guard live here instead of being copy-pasted into each plugin.

export type MdastNode = {
	type: string;
	value?: string;
	depth?: number;
	children?: MdastNode[];
	[key: string]: unknown;
};

// Narrow a node to a raw `html` node with a string `value` (the shape both
// block scanners test before reading `.value`).
export function isHtml(
	node: MdastNode | undefined,
): node is MdastNode & { value: string } {
	return !!node && node.type === "html" && typeof node.value === "string";
}
