// Private-use characters delimit inline node placeholders in Markdown
// source. Markdown syntax never matches them, but GFM autolinks run until
// whitespace, so a placeholder touching a URL lands inside the link (see
// endAutolinkAtPlaceholder).
const OPEN = "\uE000";
const CLOSE = "\uE001";
const placeholderPattern = new RegExp(`${OPEN}(\\d+)${CLOSE}`, "g");
// Markdown decodes numeric character references, so `&#xE000;` would
// produce a delimiter as surely as the character itself.
const typedDelimiterPattern = new RegExp(
	`[${OPEN}${CLOSE}]|&#(?:x0*e00[01]|0*5734[45]);`,
	"gi",
);
// remark-rehype percent-encodes link URLs.
const encodedOpenPattern = new RegExp(
	`${OPEN}|${encodeURIComponent(OPEN)}`,
	"i",
);

/** Text that stands in for an inline node inside Markdown source. */
export const inlineNodePlaceholder = (index: number) =>
	`${OPEN}${index}${CLOSE}`;

/**
 * Replaces placeholder delimiters in user-authored text, typed or written
 * as character references, so typed text can never form a placeholder.
 */
export const escapeInlineNodeDelimiters = (text: string) =>
	text.replace(typedDelimiterPattern, "\uFFFD");

/** Replaces each placeholder in `text` with the matching fallback text. */
export const replaceInlineNodePlaceholders = (
	text: string,
	fallbacks: readonly string[],
) =>
	text.replace(
		placeholderPattern,
		(match, index: string) => fallbacks[Number(index)] ?? match,
	);

type HastText = { type: "text"; value: string };
type HastElement = {
	type: "element";
	tagName: string;
	properties: Record<string, unknown>;
	children: HastChild[];
};
type HastChild = HastText | HastElement | { type: string };
type HastParent = { children: HastChild[] };

/** Property that marks a placeholder `span` with its inline node index. */
export const inlineNodeIndexProperty = "dataInlineNode";

const isText = (node: HastChild): node is HastText => node.type === "text";
const isElement = (node: HastChild): node is HastElement =>
	node.type === "element";

const splitPlaceholders = (value: string): HastChild[] => {
	const children: HastChild[] = [];
	let cursor = 0;
	for (const match of value.matchAll(placeholderPattern)) {
		if (match.index > cursor) {
			children.push({ type: "text", value: value.slice(cursor, match.index) });
		}
		children.push({
			type: "element",
			tagName: "span",
			properties: { [inlineNodeIndexProperty]: Number(match[1]) },
			children: [],
		});
		cursor = match.index + match[0].length;
	}
	if (cursor < value.length) {
		children.push({ type: "text", value: value.slice(cursor) });
	}
	return children;
};

// An autolink that swallowed a placeholder ends before it; the placeholder
// and the rest of the swallowed text follow the link.
const endAutolinkAtPlaceholder = (
	link: HastElement,
): [HastElement, ...HastChild[]] => {
	const { href } = link.properties;
	const [label] = link.children;
	if (
		typeof href !== "string" ||
		link.children.length !== 1 ||
		!isText(label)
	) {
		return [link];
	}
	const hrefCut = href.search(encodedOpenPattern);
	const labelCut = label.value.indexOf(OPEN);
	if (hrefCut <= 0 || labelCut <= 0) {
		return [link];
	}
	link.properties.href = href.slice(0, hrefCut);
	link.children = [{ type: "text", value: label.value.slice(0, labelCut) }];
	return [link, ...splitPlaceholders(label.value.slice(labelCut))];
};

const resolvePlaceholders = (parent: HastParent) => {
	parent.children = parent.children.flatMap((child): HastChild[] => {
		if (isText(child)) {
			return child.value.includes(OPEN)
				? splitPlaceholders(child.value)
				: [child];
		}
		// Fenced code renders from its raw text, which resolves placeholders
		// to fallback text instead.
		if (!isElement(child) || child.tagName === "pre") {
			return [child];
		}
		if (child.tagName !== "a") {
			resolvePlaceholders(child);
			return [child];
		}
		const [link, ...rest] = endAutolinkAtPlaceholder(child);
		resolvePlaceholders(link);
		return [link, ...rest];
	});
};

/**
 * Rehype plugin that turns placeholders outside fenced code into empty
 * `span` elements tagged with their inline node index. It runs after
 * sanitization, which would strip the index property, and before link
 * hardening, which must validate links this plugin shortens.
 */
export const rehypeInlineNodes = () => (tree: HastParent) => {
	resolvePlaceholders(tree);
};
