// Shared mdast helpers for the block-scanning remark plugins
// (remark-coder-tabs, remark-details-accordion). Both walk a node's direct
// children looking for raw-HTML delimiters, so the minimal working node shape
// and the `html` node type guard live here instead of being copy-pasted into
// each plugin.

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
