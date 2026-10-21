export type FilePathMention = {
	path: string;
	line?: number;
};

type FoundFilePathMention = FilePathMention & {
	start: number;
	end: number;
};

const bareFileExtensions = new Set([
	"go",
	"ts",
	"tsx",
	"js",
	"jsx",
	"py",
	"rs",
	"java",
	"rb",
	"md",
	"mdx",
	"json",
	"yaml",
	"yml",
	"toml",
	"sql",
	"sh",
	"proto",
	"css",
	"html",
	"txt",
]);

const extensionlessFileNames = new Set(["Makefile", "Dockerfile"]);

const pathPattern =
	"((?:~/|/)?[\\w.@+-]+(?:/[\\w.@+-]+)*)(?::(\\d+)(?::\\d+)?|#L(\\d+))?";

const wholeMentionRegExp = new RegExp(`^${pathPattern}$`);

// The lookbehind keeps matches from starting inside a word, a URL, or a
// longer path, and the lookahead keeps them from ending mid-token.
const mentionRegExp = new RegExp(
	`(?<![\\w./~@:#-])${pathPattern}(?![\\w/~@:#-])`,
	"g",
);

const isFilePath = (path: string): boolean => {
	const segments = path.split("/");
	const name = segments[segments.length - 1];
	if (!name || name.endsWith(".")) {
		return false;
	}
	const first = segments[0];
	if (segments.length > 1 && first.includes(".") && !first.startsWith(".")) {
		// The first segment looks like a domain, as in example.com/a.ts.
		return false;
	}
	if (extensionlessFileNames.has(name)) {
		return true;
	}
	const extension = /\.([A-Za-z][A-Za-z0-9]*)$/.exec(name)?.[1];
	if (!extension || name.length === extension.length + 1) {
		return false;
	}
	return segments.length > 1 || bareFileExtensions.has(extension);
};

const toMention = (
	path: string,
	line: string | undefined,
): FilePathMention | undefined => {
	if (!isFilePath(path)) {
		return undefined;
	}
	const lineNumber = line === undefined ? undefined : Number(line);
	if (lineNumber !== undefined && lineNumber > 0) {
		return { path, line: lineNumber };
	}
	return { path };
};

/**
 * Parses text that is entirely a file path, such as the contents of an
 * inline code span, with an optional `:line`, `:line:column`, or `#Lline`
 * suffix.
 */
export const parseFilePathMention = (
	text: string,
): FilePathMention | undefined => {
	const match = wholeMentionRegExp.exec(text.trim());
	if (!match) {
		return undefined;
	}
	return toMention(match[1], match[2] ?? match[3]);
};

/** Finds file path mentions in prose. */
export const findFilePathMentions = (text: string): FoundFilePathMention[] => {
	const found: FoundFilePathMention[] = [];
	for (const match of text.matchAll(mentionRegExp)) {
		let path = match[1];
		const line = match[2] ?? match[3];
		let end = match.index + match[0].length;
		if (line === undefined) {
			const trimmed = path.replace(/\.+$/, "");
			end -= path.length - trimmed.length;
			path = trimmed;
		}
		const mention = toMention(path, line);
		if (mention) {
			found.push({ ...mention, start: match.index, end });
		}
	}
	return found;
};

type HastNode = {
	type: string;
	value?: string;
	tagName?: string;
	properties?: Record<string, unknown>;
	children?: HastNode[];
};

const skippedTagNames = new Set(["a", "code", "pre", "button"]);

const splitMentions = (node: HastNode): HastNode[] | undefined => {
	const value = node.value ?? "";
	const mentions = findFilePathMentions(value);
	if (mentions.length === 0) {
		return undefined;
	}
	const nodes: HastNode[] = [];
	let offset = 0;
	for (const mention of mentions) {
		if (mention.start > offset) {
			nodes.push({ type: "text", value: value.slice(offset, mention.start) });
		}
		nodes.push({
			type: "element",
			tagName: "span",
			properties: {
				dataFileMention: mention.path,
				dataFileMentionLine: mention.line,
			},
			children: [
				{ type: "text", value: value.slice(mention.start, mention.end) },
			],
		});
		offset = mention.end;
	}
	if (offset < value.length) {
		nodes.push({ type: "text", value: value.slice(offset) });
	}
	return nodes;
};

const visit = (node: HastNode) => {
	if (!node.children) {
		return;
	}
	const children: HastNode[] = [];
	for (const child of node.children) {
		if (child.type === "text") {
			const split = splitMentions(child);
			children.push(...(split ?? [child]));
			continue;
		}
		if (child.type !== "element" || !skippedTagNames.has(child.tagName ?? "")) {
			visit(child);
		}
		children.push(child);
	}
	node.children = children;
};

/**
 * Rehype plugin that wraps file path mentions in prose with
 * `<span data-file-mention="path" data-file-mention-line="n">`. Text in
 * links and code is left alone; inline code is matched separately.
 */
export const rehypeFileMentions = () => visit;
