import { fromMarkdown } from "mdast-util-from-markdown";
import { gfmAutolinkLiteralFromMarkdown } from "mdast-util-gfm-autolink-literal";
import { gfmAutolinkLiteral } from "micromark-extension-gfm-autolink-literal";
import { visit } from "unist-util-visit";

type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

/**
 * Concatenating the returned segment values reproduces the input.
 * Streamdown uses the same GFM autolink parser; slicing its ranges keeps
 * prompt markdown literal, and only explicit http(s) literals become URLs.
 */
export const splitTextForLinks = (text: string): LinkSegment[] => {
	const tree = fromMarkdown(text, {
		extensions: [gfmAutolinkLiteral()],
		mdastExtensions: [gfmAutolinkLiteralFromMarkdown()],
	});
	const segments: LinkSegment[] = [];
	let lastIndex = 0;
	visit(tree, "link", (node) => {
		const start = node.position?.start.offset;
		const end = node.position?.end.offset;
		if (start === undefined || end === undefined) {
			return;
		}
		const value = text.slice(start, end);
		// Autolink literals span exactly the URL text. The scheme check
		// excludes email and www autolinks plus [text](url) nodes, whose
		// slices would leak markdown syntax.
		if (!/^https?:\/\//i.test(value)) {
			return;
		}
		if (start > lastIndex) {
			segments.push({ kind: "text", value: text.slice(lastIndex, start) });
		}
		segments.push({ kind: "url", value });
		lastIndex = end;
	});
	if (lastIndex < text.length) {
		segments.push({ kind: "text", value: text.slice(lastIndex) });
	}
	return segments;
};
