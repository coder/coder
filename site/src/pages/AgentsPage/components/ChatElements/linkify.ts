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
		// Angle-bracket autolinks (<url>) include the brackets in the node
		// range; keep the brackets as text and slice only the inner URL.
		let urlStart = start;
		let urlEnd = end;
		if (text[start] === "<" && text[end - 1] === ">") {
			urlStart += 1;
			urlEnd -= 1;
		}
		const value = text.slice(urlStart, urlEnd);
		// Autolink literals span exactly the URL text. The scheme check
		// excludes email and www autolinks plus [text](url) nodes, whose
		// slices would leak markdown syntax.
		if (!/^https?:\/\//i.test(value)) {
			return;
		}
		if (urlStart > lastIndex) {
			segments.push({ kind: "text", value: text.slice(lastIndex, urlStart) });
		}
		segments.push({ kind: "url", value });
		lastIndex = urlEnd;
	});
	if (lastIndex < text.length) {
		segments.push({ kind: "text", value: text.slice(lastIndex) });
	}
	return segments;
};
