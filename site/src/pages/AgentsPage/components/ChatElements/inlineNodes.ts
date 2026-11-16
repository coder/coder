// Why this file exists: a user message is text with chips (file and skill
// links) mixed in. If we render each piece of text on its own, every piece
// becomes its own paragraph and the chip drops onto its own line. So we
// join everything into one Markdown string, put a hidden marker where each
// chip goes, and swap the real chip back in after the Markdown is rendered.

/** A chip to show inside Markdown text. */
export type InlineNode = {
	element: React.ReactNode;
	/** Plain text to show instead when the chip is inside a code block. */
	text: string;
};

// The marker uses two invisible characters nobody types, so Markdown
// leaves it alone.
const OPEN = "\uE000";
const CLOSE = "\uE001";
const placeholderPattern = new RegExp(`${OPEN}(\\d+)${CLOSE}`, "g");
// Users could type a marker, either directly or as an HTML code like
// `&#xE000;`. This finds both so we can remove them.
const typedDelimiterPattern = new RegExp(
	`[${OPEN}${CLOSE}]|&#(?:x0*e00[01]|0*5734[45]);`,
	"gi",
);

/**
 * Joins text and chips into one Markdown string, with a marker for each
 * chip. Markers the user typed are removed so they can't fake a chip.
 */
export const joinInlineNodes = (
	parts: readonly (string | InlineNode)[],
): { markdown: string; inlineNodes: InlineNode[] } => {
	const inlineNodes: InlineNode[] = [];
	let markdown = "";
	for (const part of parts) {
		if (typeof part === "string") {
			markdown += part.replace(typedDelimiterPattern, "\uFFFD");
		} else {
			markdown += `${OPEN}${inlineNodes.length}${CLOSE}`;
			inlineNodes.push(part);
		}
	}
	return { markdown, inlineNodes };
};

/** Replaces markers with each chip's plain text, for code blocks. */
export const resolveInlineNodeText = (
	text: string,
	inlineNodes: readonly InlineNode[],
) =>
	text.replace(
		placeholderPattern,
		(match, index: string) => inlineNodes[Number(index)]?.text ?? match,
	);

/** Tells Response which chip a marker span stands for. */
export const inlineNodeIndexProperty = "dataInlineNode";

type HastNode = {
	type: string;
	value?: string;
	tagName?: string;
	properties?: Record<string, unknown>;
	children?: HastNode[];
};

const text = (value: string): HastNode => ({ type: "text", value });

// Keep the marker text inside the span so code blocks, which read plain
// text, can still swap in the chip's text.
const splitPlaceholders = (value: string): HastNode[] =>
	value.split(placeholderPattern).flatMap((piece, i) => {
		if (i % 2 === 0) {
			return piece ? [text(piece)] : [];
		}
		return {
			type: "element",
			tagName: "span",
			properties: { [inlineNodeIndexProperty]: Number(piece) },
			children: [text(`${OPEN}${piece}${CLOSE}`)],
		};
	});

// A bare URL becomes a link that runs until the next space, so a chip
// typed right after a URL ends up inside the link. This cuts the link off
// where the chip starts.
const endAutolinkAtPlaceholder = (link: HastNode): HastNode[] => {
	const href = link.properties?.href;
	const label =
		link.children?.length === 1 ? link.children[0].value : undefined;
	if (typeof href !== "string" || label === undefined) {
		return [link];
	}
	// The link's address is URL-encoded, so look for the encoded marker.
	const hrefCut = href.toUpperCase().indexOf(encodeURIComponent(OPEN));
	const labelCut = label.indexOf(OPEN);
	if (hrefCut <= 0 || labelCut <= 0) {
		return [link];
	}
	link.properties = { ...link.properties, href: href.slice(0, hrefCut) };
	link.children = [text(label.slice(0, labelCut))];
	return [link, ...splitPlaceholders(label.slice(labelCut))];
};

const resolvePlaceholders = (parent: HastNode) => {
	if (!parent.children) {
		return;
	}
	parent.children = parent.children.flatMap((child) => {
		if (child.type === "text") {
			return child.value?.includes(OPEN)
				? splitPlaceholders(child.value)
				: [child];
		}
		const nodes =
			child.tagName === "a" ? endAutolinkAtPlaceholder(child) : [child];
		resolvePlaceholders(child);
		return nodes;
	});
};

/**
 * Finds the markers in rendered Markdown and turns each into a span that
 * Response replaces with the real chip.
 */
export const rehypeInlineNodes = () => (tree: HastNode) => {
	resolvePlaceholders(tree);
};
