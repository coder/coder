import { Fragment } from "react";

const LINK_PATTERN =
	/\[([^\]\n]+)\]\(\s*<((?:https?:\/\/|mailto:|www\.)[^>\s]+)>\s*\)|\[([^\]\n]+)\]\(\s*((?:https?:\/\/|mailto:|www\.)[^\s)]+)\s*\)|<((?:https?:\/\/|mailto:|www\.)[^>\s]+)>|((?:https?:\/\/|mailto:|www\.)[^\s<>"']+)/gi;

const TRAILING_PUNCTUATION = new Set([".", ",", "!", "?", ";", ":"]);
const CLOSING_PAIRS: Record<string, string> = {
	")": "(",
	"]": "[",
	"}": "{",
};

type TextSegment = {
	type: "text";
	text: string;
};

type LinkSegment = {
	type: "link";
	text: string;
	href: string;
};

type LinkifiedTextSegment = TextSegment | LinkSegment;

const countChar = (value: string, char: string): number => {
	let count = 0;
	for (const current of value) {
		if (current === char) {
			count += 1;
		}
	}
	return count;
};

const trimTrailingUrlText = (value: string) => {
	let linkText = value;
	let trailingText = "";

	while (linkText.length > 0) {
		const lastChar = linkText[linkText.length - 1];
		const matchingOpen = CLOSING_PAIRS[lastChar];

		if (TRAILING_PUNCTUATION.has(lastChar)) {
			trailingText = `${lastChar}${trailingText}`;
			linkText = linkText.slice(0, -1);
			continue;
		}

		if (
			matchingOpen &&
			countChar(linkText, lastChar) > countChar(linkText, matchingOpen)
		) {
			trailingText = `${lastChar}${trailingText}`;
			linkText = linkText.slice(0, -1);
			continue;
		}

		break;
	}

	return { linkText, trailingText };
};

const toHref = (url: string): string => {
	if (url.toLowerCase().startsWith("www.")) {
		return `https://${url}`;
	}
	return url;
};

const appendTextSegment = (segments: LinkifiedTextSegment[], text: string) => {
	if (text.length === 0) {
		return;
	}
	const previous = segments[segments.length - 1];
	if (previous?.type === "text") {
		previous.text += text;
		return;
	}
	segments.push({ type: "text", text });
};

export const parseLinkifiedText = (text: string): LinkifiedTextSegment[] => {
	const segments: LinkifiedTextSegment[] = [];
	let lastIndex = 0;

	for (const match of text.matchAll(LINK_PATTERN)) {
		const matchText = match[0];
		const matchIndex = match.index ?? 0;
		const label = match[1] ?? match[3];
		const markdownURL = match[2] ?? match[4];
		const angleURL = match[5];
		const bareURL = match[6];
		const previousChar = matchIndex > 0 ? text[matchIndex - 1] : "";

		if (label && previousChar === "!") {
			continue;
		}

		appendTextSegment(segments, text.slice(lastIndex, matchIndex));

		if (markdownURL) {
			segments.push({ type: "link", text: label, href: toHref(markdownURL) });
			lastIndex = matchIndex + matchText.length;
			continue;
		}

		const { linkText, trailingText } = trimTrailingUrlText(angleURL ?? bareURL);
		segments.push({ type: "link", text: linkText, href: toHref(linkText) });
		appendTextSegment(segments, trailingText);
		lastIndex = matchIndex + matchText.length;
	}

	appendTextSegment(segments, text.slice(lastIndex));
	return segments;
};

export const LinkifiedText = ({ text }: { text: string }) => {
	return (
		<>
			{parseLinkifiedText(text).map((segment, index) => {
				if (segment.type === "text") {
					return <Fragment key={index}>{segment.text}</Fragment>;
				}

				return (
					<a
						key={index}
						href={segment.href}
						target="_blank"
						rel="noopener noreferrer"
						className="text-content-link no-underline hover:underline hover:decoration-content-link"
					>
						{segment.text}
					</a>
				);
			})}
		</>
	);
};
