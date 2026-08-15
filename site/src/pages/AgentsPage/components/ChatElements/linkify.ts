import { find } from "linkifyjs";

type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

// Reject bare-domain matches: linkifyjs would otherwise linkify filenames
// like README.md, since .md is a TLD.
const options = {
	validate: { url: (value: string) => /^https?:\/\//i.test(value) },
};

/** Concatenating the returned segment values reproduces the input. */
export const splitTextForLinks = (text: string): LinkSegment[] => {
	const segments: LinkSegment[] = [];
	let lastIndex = 0;
	for (const link of find(text, "url", options)) {
		if (link.start > lastIndex) {
			segments.push({
				kind: "text",
				value: text.slice(lastIndex, link.start),
			});
		}
		segments.push({ kind: "url", value: link.value });
		lastIndex = link.end;
	}
	if (lastIndex < text.length) {
		segments.push({ kind: "text", value: text.slice(lastIndex) });
	}
	return segments;
};
