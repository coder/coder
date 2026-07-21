// Preserve fenced-code meta (e.g. `title="main.tf"`) through the rehype-raw
// pass so Fumadocs can render code-block titles on plain-Markdown (`.md`)
// pages.
//
// Why this is needed: remark-rehype stores a code fence's meta string on the
// generated `<code>` node's `data.meta`. Our pipeline then runs `rehype-raw`
// (to keep raw HTML like tables), which serializes the tree to HTML and
// re-parses it; `data.*` fields are not HTML attributes, so `data.meta` is
// dropped before Fumadocs' `rehypeCode` ever sees it. As a result `title=`,
// line-number, and other meta silently do nothing.
//
// Fumadocs' `rehypeCode` reads the meta from EITHER `code.data.meta` OR the
// `<code>` element's `metastring` property. `metastring` is a real attribute,
// so it survives the rehype-raw round-trip. This plugin copies each code
// fence's `meta` into `data.hProperties.metastring`, which mdast-util-to-hast
// applies to the `<code>` element. That is the whole fix: meta now reaches the
// highlighter, so `title="..."` renders a filename bar.
//
// Blast radius is intentionally zero for existing content: the corpus has no
// code fences with meta today, so this only affects fences that are explicitly
// given meta (currently the single-file "complete code" disclosures).

type MdastNode = {
	type: string;
	meta?: string | null;
	data?: {
		hProperties?: Record<string, unknown>;
		[key: string]: unknown;
	};
	children?: MdastNode[];
	[key: string]: unknown;
};

function visit(node: MdastNode): void {
	if (
		node.type === "code" &&
		typeof node.meta === "string" &&
		node.meta.trim() !== ""
	) {
		const data = (node.data ??= {});
		const hProperties = (data.hProperties ??= {});
		// Don't clobber an explicit metastring if one is somehow already present.
		if (hProperties.metastring === undefined) {
			hProperties.metastring = node.meta;
		}
	}
	if (node.children) {
		for (const child of node.children) visit(child);
	}
}

export function remarkCodeMeta() {
	return (tree: MdastNode): void => {
		visit(tree);
	};
}
