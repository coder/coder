type FilePathMention = {
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

type TextRange = {
	start: number;
	end: number;
};

// How far past a path-like token the filesystem may extend a mention.
const candidateTailLimit = 160;

// A path-like token followed by a space and more text, as in
// /path/to/file (2).txt, where only the filesystem knows where it ends.
const candidateTokenRegExp =
	/(?<![\w./~@:#\\\])}-])((?:~\/|\/)?[\w.+-]+(?:\/[\w.+-]+)*) (?=\S)/g;

const isPathLike = (token: string) =>
	token.startsWith("/") || token.startsWith("~/") || token.includes("/");

const findCandidates = (
	text: string,
	mentions: readonly TextRange[],
): TextRange[] => {
	const found: TextRange[] = [];
	for (const match of text.matchAll(candidateTokenRegExp)) {
		const token = match[1];
		const start = match.index;
		const previous = found[found.length - 1];
		if (!isPathLike(token) || (previous && start < previous.end)) {
			continue;
		}
		const before = text[start - 1];
		if (
			/^[/~]/.test(token) &&
			before !== undefined &&
			!rootedMentionBoundary.test(before)
		) {
			continue;
		}
		const first = token.split("/")[0];
		if (first.includes(".") && !first.startsWith(".")) {
			continue;
		}
		const lineEnd = text.indexOf("\n", start);
		let end = Math.min(
			lineEnd === -1 ? text.length : lineEnd,
			start + token.length + candidateTailLimit,
		);
		for (const mention of mentions) {
			if (mention.start < end && mention.end > end) {
				end = mention.end;
			}
		}
		found.push({ start, end });
	}
	return found;
};

/**
 * Finds ranges of prose that start with a path-like token followed by a
 * space, so the filesystem can decide how far the path extends. Each range
 * runs to the end of the line, capped, and covers any mention it overlaps.
 */
export const findFilePathCandidates = (text: string): TextRange[] =>
	findCandidates(text, findFilePathMentions(text));

/**
 * Reports whether inline code text may be a path containing spaces. The
 * code span bounds the path, so verification must consume all of it.
 */
export const isWholeFilePathCandidate = (text: string): boolean =>
	text.includes(" ") &&
	text.length <= candidateTailLimit &&
	isPathLike(text) &&
	!/[\n"'`$|&;<>=*?]|:\/\//.test(text);

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
	if (text?.type !== "text") {
		return;
	}
	const value = text.value ?? "";
	const mention = parseFilePathMention(value);
	if (mention) {
		node.properties ??= {};
		markMention(node.properties, mention);
	} else if (isWholeFilePathCandidate(value)) {
		node.properties ??= {};
		node.properties.dataFileMentionCandidate = value;
	}
};

const splitRange = (
	value: string,
	range: TextRange,
	mentions: readonly FoundFilePathMention[],
): HastNode[] => {
	const nodes: HastNode[] = [];
	let offset = range.start;
	for (const mention of mentions) {
		if (mention.start < range.start || mention.end > range.end) {
			continue;
		}
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
	if (offset < range.end) {
		nodes.push({ type: "text", value: value.slice(offset, range.end) });
	}
	return nodes;
};

const splitMentions = (node: HastNode): HastNode[] | undefined => {
	const value = node.value ?? "";
	const mentions = findFilePathMentions(value);
	const candidates = findCandidates(value, mentions);
	if (mentions.length === 0 && candidates.length === 0) {
		return undefined;
	}
	const nodes: HastNode[] = [];
	let offset = 0;
	for (const candidate of candidates) {
		nodes.push(
			...splitRange(value, { start: offset, end: candidate.start }, mentions),
		);
		nodes.push({
			type: "element",
			tagName: "span",
			properties: {
				dataFileMentionCandidate: value.slice(candidate.start, candidate.end),
			},
			children: splitRange(value, candidate, mentions),
		});
		offset = candidate.end;
	}
	nodes.push(
		...splitRange(value, { start: offset, end: value.length }, mentions),
	);
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
 * path, and on spans it wraps around paths in prose. Text that may be a path
 * with spaces gets `data-file-mention-candidate` with the text to verify
 * instead. Links and code blocks are left alone.
 */
export const rehypeFileMentions = () => (tree: HastNode) => visit(tree);

/** Where a candidate's directory walk begins. */
type FilePathCandidateStart = {
	/** Directory to list first, in a form the list-directory API accepts. */
	directory: string;
	/** Index in the candidate text where the first entry name begins. */
	offset: number;
};

const joinPath = (base: string, path: string) => {
	if (!base) {
		return path;
	}
	if (!path) {
		return base;
	}
	return `${base.replace(/\/$/, "")}/${path}`;
};

/**
 * Lists the directories a candidate can start from. The text before the
 * first space has no ambiguity, so the walk starts in the directory that
 * its last slash names. Relative text starts in each repository root, or
 * the agent's default directory (`""`) when the chat has none.
 */
export const getFilePathCandidateStarts = (
	text: string,
	repoRoots: readonly string[],
): FilePathCandidateStart[] => {
	const space = text.indexOf(" ");
	const token = space === -1 ? text : text.slice(0, space);
	const slash = token.lastIndexOf("/");
	const offset = slash + 1;
	const prefix = slash === -1 ? "" : token.slice(0, slash);
	if (token.startsWith("/")) {
		return [{ directory: prefix || "/", offset }];
	}
	if (token.startsWith("~/")) {
		return [{ directory: prefix, offset }];
	}
	const roots = repoRoots.length > 0 ? repoRoots : [""];
	return roots.map((root) => ({ directory: joinPath(root, prefix), offset }));
};

type DirectoryEntry = {
	name: string;
	is_dir: boolean;
	absolute_path_string: string;
};

export type VerifiedFilePath = {
	absolutePath: string;
	/** Characters of the walked text that the path and line suffix cover. */
	length: number;
	line?: number;
};

// Bounds the list-directory calls for one candidate start.
const maxWalkDepth = 5;

const isPathEnd = (text: string, index: number) => {
	const next = text[index];
	if (next === undefined || /[\s,;:!?)\]}"'>#]/.test(next)) {
		return true;
	}
	return (
		next === "." && (index + 1 === text.length || /\s/.test(text[index + 1]))
	);
};

const lineSuffixRegExp = /^(?::(\d+)(?::\d+)?|#L(\d+))/;

/**
 * Walks directory listings to find the longest file path that `text`
 * starts with, beginning in `directory`. At each level it takes the
 * longest entry name the remaining text starts with, followed by `/` for a
 * directory or a word boundary for a file. Resolves to null when no file
 * matches, a listing fails, or the walk exceeds its depth.
 */
export const verifyFilePath = async (
	directory: string,
	text: string,
	listDirectory: (directory: string) => Promise<readonly DirectoryEntry[]>,
): Promise<VerifiedFilePath | null> => {
	let current = directory;
	let offset = 0;
	for (let depth = 0; depth < maxWalkDepth; depth++) {
		const entries = await listDirectory(current).catch(() => null);
		if (!entries) {
			return null;
		}
		const rest = text.slice(offset);
		let best: DirectoryEntry | undefined;
		for (const entry of entries) {
			const length = entry.name.length;
			if (
				length === 0 ||
				(best && length <= best.name.length) ||
				!rest.startsWith(entry.name)
			) {
				continue;
			}
			const fits = entry.is_dir
				? rest[length] === "/"
				: isPathEnd(rest, length);
			if (fits) {
				best = entry;
			}
		}
		if (!best) {
			return null;
		}
		if (best.is_dir) {
			current = best.absolute_path_string;
			offset += best.name.length + 1;
			continue;
		}
		const end = offset + best.name.length;
		const suffix = lineSuffixRegExp.exec(text.slice(end));
		const line = Number(suffix?.[1] ?? suffix?.[2]);
		if (suffix && line > 0 && isPathEnd(text, end + suffix[0].length)) {
			return {
				absolutePath: best.absolute_path_string,
				length: end + suffix[0].length,
				line,
			};
		}
		return { absolutePath: best.absolute_path_string, length: end };
	}
	return null;
};
