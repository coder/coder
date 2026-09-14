const DEFAULT_THINKING_TITLE = "Thinking";

type LineRange = {
	line: string;
	start: number;
	nextStart: number;
};

type HeadingMatch = {
	text: string;
	start: number;
	end: number;
};

type ThinkingDisclosureDisplay = {
	title: string;
	body: string;
};

const cleanHeadingText = (text: string): string =>
	text
		.replace(/\\([\\`*_[\]{}()#+.!-])/g, "$1")
		.replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
		.replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
		.replace(/`([^`]*)`/g, "$1")
		.replace(/\*\*([^*]+)\*\*/g, "$1")
		.replace(/__([^_]+)__/g, "$1")
		.replace(/\*([^*]+)\*/g, "$1")
		.replace(/_([^_]+)_/g, "$1")
		.replace(/~~([^~]+)~~/g, "$1")
		.replace(/<\/?[^>]+>/g, "")
		.replace(/\s+/g, " ")
		.trim();

const getLines = (text: string): LineRange[] => {
	const lines: LineRange[] = [];
	const linePattern = /[^\r\n]*(?:\r\n|\r|\n|$)/g;

	for (const match of text.matchAll(linePattern)) {
		const rawLine = match[0];
		const start = match.index ?? 0;
		if (rawLine === "" && start === text.length) {
			break;
		}

		const line = rawLine.replace(/\r\n$|\r$|\n$/, "");
		lines.push({
			line,
			start,
			nextStart: start + rawLine.length,
		});
	}

	return lines;
};

const getAtxHeadingText = (line: string): string | undefined => {
	const match = line.match(/^ {0,3}#{1,6}(?:[ \t]+|$)(.*)$/);
	if (!match) {
		return undefined;
	}

	const heading = cleanHeadingText(match[1].replace(/[ \t]+#{1,}[ \t]*$/, ""));
	return heading || undefined;
};

const getFenceMarker = (
	line: string,
): { character: "`" | "~"; length: number } | undefined => {
	const match = line.match(/^ {0,3}(`{3,}|~{3,})/);
	if (!match) {
		return undefined;
	}

	const marker = match[1];
	return {
		character: marker[0] as "`" | "~",
		length: marker.length,
	};
};

const isClosingFence = (
	line: string,
	marker: { character: "`" | "~"; length: number },
): boolean => {
	const match = line.match(/^ {0,3}(`{3,}|~{3,})[ \t]*$/);
	return (
		!!match &&
		match[1][0] === marker.character &&
		match[1].length >= marker.length
	);
};

const hasBodyAfterLine = (
	lines: readonly LineRange[],
	index: number,
): boolean => lines.slice(index + 1).some(({ line }) => line.trim().length > 0);

const getEmphasizedLineHeadingText = (line: string): string | undefined => {
	const match = line.match(/^ {0,3}(?:\*\*([^*]+)\*\*|__([^_]+)__)[ \t]*$/);
	if (!match) {
		return undefined;
	}

	const heading = cleanHeadingText(match[1] ?? match[2] ?? "");
	return heading || undefined;
};

const isHeadingLikeParagraph = (
	lines: readonly LineRange[],
	index: number,
	text: string,
): string | undefined => {
	const lineRange = lines[index];
	const prefix = text.slice(0, lineRange.start);
	if (prefix.trim()) {
		return undefined;
	}

	const emphasizedHeading = getEmphasizedLineHeadingText(lineRange.line);
	const nextLine = lines[index + 1];
	const hasBody = hasBodyAfterLine(lines, index);
	if ((!nextLine || nextLine.line.trim() || !hasBody) && !emphasizedHeading) {
		return undefined;
	}

	const heading = emphasizedHeading ?? cleanHeadingText(lineRange.line);
	if (!heading) {
		return undefined;
	}

	const wordCount = heading.split(/\s+/).length;
	if (heading.length > 96 || wordCount > 12 || /[.!?]$/.test(heading)) {
		return undefined;
	}

	if (
		/^[a-z]/.test(heading) ||
		/^(I|I'm|I’m|We|We're|We’re|Let's|Let’s)\b/.test(heading)
	) {
		return undefined;
	}

	return heading;
};

const getFirstHeading = (text: string): HeadingMatch | undefined => {
	let activeFence: { character: "`" | "~"; length: number } | undefined;
	let setextCandidate: LineRange | undefined;
	const lines = getLines(text);

	for (const [index, lineRange] of lines.entries()) {
		const { line } = lineRange;
		if (activeFence) {
			if (isClosingFence(line, activeFence)) {
				activeFence = undefined;
			}
			continue;
		}

		const openingFence = getFenceMarker(line);
		if (openingFence) {
			activeFence = openingFence;
			setextCandidate = undefined;
			continue;
		}

		const atxHeading = getAtxHeadingText(line);
		if (atxHeading) {
			return {
				text: atxHeading,
				start: lineRange.start,
				end: lineRange.nextStart,
			};
		}

		const paragraphHeading = isHeadingLikeParagraph(lines, index, text);
		if (paragraphHeading) {
			return {
				text: paragraphHeading,
				start: lineRange.start,
				end: lineRange.nextStart,
			};
		}

		if (/^ {0,3}(=+|-+)[ \t]*$/.test(line) && setextCandidate) {
			const heading = cleanHeadingText(setextCandidate.line);
			if (!heading) {
				return undefined;
			}
			return {
				text: heading,
				start: setextCandidate.start,
				end: lineRange.nextStart,
			};
		}

		const trimmedLine = line.trim();
		setextCandidate = trimmedLine ? lineRange : undefined;
	}

	return undefined;
};

const lowercaseSentenceStart = (text: string): string => {
	const firstWord = text.match(/^[A-Za-z]+\b/)?.[0];
	if (!firstWord || !/^[A-Z][a-z]+$/.test(firstWord)) {
		return text;
	}

	return `${firstWord[0].toLowerCase()}${text.slice(1)}`;
};

const removeHeading = (text: string, heading: HeadingMatch): string => {
	const beforeHeading = text.slice(0, heading.start);
	const body = `${beforeHeading}${text.slice(heading.end)}`;
	if (beforeHeading.trim()) {
		return body;
	}
	return body.replace(/^\s+/, "");
};

export const getThinkingDisclosureDisplay = (
	text: string,
): ThinkingDisclosureDisplay => {
	const heading = getFirstHeading(text);
	if (!heading) {
		return {
			title: DEFAULT_THINKING_TITLE,
			body: text,
		};
	}

	return {
		title: `${DEFAULT_THINKING_TITLE} about ${lowercaseSentenceStart(heading.text)}`,
		body: removeHeading(text, heading),
	};
};
