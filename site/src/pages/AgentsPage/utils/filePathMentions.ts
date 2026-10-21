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

// Sites whose bare names look like source files.
const domainNames = new Set(["docs.rs", "bun.sh", "rustup.rs", "starship.rs"]);

const buildPathPattern = (segmentChars: string) =>
	`((?:~/|/)?[${segmentChars}]+(?:/[${segmentChars}]+)*)(?::(\\d+)(?::\\d+)?|#L(\\d+))?`;

// Inline code holds nothing but the path, so segments may contain
// characters that would be ambiguous in prose, as in src/app/(auth)/[id].tsx.
const wholeMentionRegExp = new RegExp(
	`^${buildPathPattern("\\w.@+()\\[\\]-")}$`,
);

// The lookbehind keeps matches from starting inside a word, a URL, or a
// longer path-like token, and the lookahead keeps them from ending mid-token.
const mentionRegExp = new RegExp(
	`(?<![\\w./~@:#\\\\\\])}-])${buildPathPattern("\\w.+-")}(?![\\w/~@:#\\\\(\\[-])`,
	"g",
);

// A rooted match must start a token, so the tail of a longer path is never
// read as an absolute path.
const rootedMentionBoundary = /[\s("'`<]/;

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
	if (segments.length > 1) {
		return true;
	}
	// Framework names such as Node.js, addresses, and sites read as prose.
	if (
		/^[A-Z][a-z0-9]*\.js$/.test(name) ||
		name.includes("@") ||
		domainNames.has(name)
	) {
		return false;
	}
	return bareFileExtensions.has(extension);
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
		const previous = text[match.index - 1];
		if (
			/^[/~]/.test(path) &&
			previous !== undefined &&
			!rootedMentionBoundary.test(previous)
		) {
			continue;
		}
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

const skippedTagNames = new Set(["a", "pre", "button"]);

const markMention = (
	properties: Record<string, unknown>,
	mention: FilePathMention,
) => {
	properties.dataFileMention = mention.path;
	properties.dataFileMentionLine = mention.line;
};

const markInlineCode = (node: HastNode) => {
	const text = node.children?.length === 1 ? node.children[0] : undefined;
	const mention =
		text?.type === "text" ? parseFilePathMention(text.value ?? "") : undefined;
	if (mention) {
		node.properties ??= {};
		markMention(node.properties, mention);
	}
};

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
		const properties: Record<string, unknown> = {};
		markMention(properties, mention);
		nodes.push({
			type: "element",
			tagName: "span",
			properties,
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
		if (child.type === "element" && child.tagName === "code") {
			markInlineCode(child);
		} else if (
			child.type === "element" &&
			!skippedTagNames.has(child.tagName ?? "")
		) {
			visit(child);
		}
		children.push(child);
	}
	node.children = children;
};

/**
 * Rehype plugin that marks file path mentions with `data-file-mention` and
 * `data-file-mention-line` properties: on inline code that is entirely a
 * path, and on spans it wraps around paths in prose. Links and code blocks
 * are left alone.
 */
export const rehypeFileMentions = () => (tree: HastNode) => visit(tree);
