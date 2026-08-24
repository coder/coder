// Shared mdast node shape and type guards for the remark plugins here.

export type MdastNode = {
	type: string;
	value?: string;
	depth?: number;
	children?: MdastNode[];
	[key: string]: unknown;
};

export function isHtml(
	node: MdastNode | undefined,
): node is MdastNode & { value: string } {
	return !!node && node.type === "html" && typeof node.value === "string";
}
