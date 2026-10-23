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
	// Dotfiles such as .gitignore and .env.local have no other name part;
	// a bare extension such as .go is not one.
	if (
		/^\.[A-Za-z0-9_][\w.-]*$/.test(name) &&
		!bareFileExtensions.has(name.slice(1))
	) {
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
// /path/to/file (2).txt, where only the filesystem knows where it ends, or
// a token ending in a slash, as in site/src/, that may name a folder.
const candidateTokenRegExp =
	/(?<![\w./~@:#\\\])}-])((?:~\/|\/)?[\w.+-]+(?:\/[\w.+-]+)*)(?: (?=\S)|(\/)(?=[\s,;:!?)\]}"'>]|\.?$|\.\s))/g;

const isPathLike = (token: string) =>
	token.startsWith("/") || token.startsWith("~/") || token.includes("/");

type CandidateToken = {
	start: number;
	token: string;
};

const findCandidateTokens = (text: string): CandidateToken[] => {
	const tokens: CandidateToken[] = [];
	for (const match of text.matchAll(candidateTokenRegExp)) {
		const token = match[1];
		const start = match.index;
		if (!isPathLike(token) && !match[2]) {
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
		tokens.push({ start, token });
	}
	return tokens;
};

/**
 * Lists where a path with spaces could start in `text`, including starts
 * inside another candidate's tail, as in "a/my x.md and b/my y.md".
 */
export const findCandidateTokenStarts = (text: string): number[] =>
	findCandidateTokens(text).map(({ start }) => start);

/**
 * Where a candidate path the filesystem rejected could have ended: after
 * the first word that ends in a file extension, or at the end of the text.
 * Another candidate may start only after it, so words of a rejected path,
 * as in "/missing/my documents/notes (2).md", never become a path of their
 * own.
 */
export const findRejectedCandidateEnd = (text: string): number => {
	for (const match of text.matchAll(/\S+/g)) {
		if (/\.[A-Za-z][A-Za-z0-9]*[)\]}"'.,;:!?]*$/.test(match[0])) {
			return match.index + match[0].length;
		}
	}
	return text.length;
};

const findCandidates = (
	text: string,
	mentions: readonly TextRange[],
): TextRange[] => {
	const found: TextRange[] = [];
	for (const { start, token } of findCandidateTokens(text)) {
		const previous = found[found.length - 1];
		if (previous && start < previous.end) {
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
 * Reports whether inline code text may be a path containing spaces, or a
 * folder path ending in a slash. The code span bounds the path, so
 * verification must consume all of it.
 */
export const isWholeFilePathCandidate = (text: string): boolean =>
	(text.includes(" ") || text.endsWith("/")) &&
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

const markInlineCode = (node: HastNode) => {
	const text = node.children?.length === 1 ? node.children[0] : undefined;
	if (text?.type !== "text") {
		return;
	}
	const value = text.value ?? "";
	if (parseFilePathMention(value)) {
		node.properties ??= {};
		node.properties.dataFileMentionCandidate = value.trim();
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
		const text = value.slice(mention.start, mention.end);
		nodes.push({
			type: "element",
			tagName: "span",
			properties: {
				dataFileMentionCandidate: text,
				dataFileMentionWhole: true,
			},
			children: [{ type: "text", value: text }],
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
				dataFileMentionCandidateOpener: value[candidate.start - 1],
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
 * Rehype plugin that marks text to verify as a path with
 * `data-file-mention-candidate`: inline code that is entirely a path or may
 * be one with spaces, spans it wraps around paths in prose, which also get
 * `data-file-mention-whole`, and spans around prose that may start a path
 * with spaces, which also get `data-file-mention-candidate-opener` with the
 * character before them. Links and code blocks are left alone.
 */
export const rehypeFileMentions = () => (tree: HastNode) => visit(tree);

/** Where a candidate's directory walk begins. */
type FilePathCandidateStart = {
	/**
	 * Directory the walk lists first and a relative path resolves in, in a
	 * form the list-directory API accepts: `/`, `~`, a repository root, or
	 * the agent's default directory (`""`).
	 */
	root: string;
	/** Index in the candidate text where the first entry name begins. */
	offset: number;
	/** A match here counts only when no repository root matches. */
	fallback?: boolean;
};

/**
 * Lists the roots a candidate's walk can start from. The walk starts at a
 * root and lists only directories that an earlier listing showed, so it
 * never requests a directory that does not exist. Relative text starts in
 * each repository root, then the agent's default directory as a fallback,
 * or the default directory (`""`) alone when the chat has no repository.
 */
export const getFilePathCandidateStarts = (
	text: string,
	repoRoots: readonly string[],
	defaultDirectory?: string,
): FilePathCandidateStart[] => {
	if (text.startsWith("/")) {
		return [{ root: "/", offset: 1 }];
	}
	if (text.startsWith("~/")) {
		return [{ root: "~", offset: 2 }];
	}
	if (repoRoots.length === 0) {
		return [{ root: "", offset: 0 }];
	}
	const starts: FilePathCandidateStart[] = repoRoots.map((root) => ({
		root,
		offset: 0,
	}));
	if (defaultDirectory && !repoRoots.includes(defaultDirectory)) {
		starts.push({ root: defaultDirectory, offset: 0, fallback: true });
	}
	return starts;
};

type DirectoryEntry = {
	name: string;
	is_dir: boolean;
	absolute_path_string: string;
};

type VerifiedFilePath = {
	absolutePath: string;
	/** Characters of the walked text that the path and line suffix cover. */
	length: number;
	line?: number;
	isDirectory?: boolean;
};

// Bounds the list-directory calls for one candidate start. The walk lists
// every level from the root, so the bound covers deep absolute paths.
const maxWalkDepth = 24;

const isPathEnd = (text: string, index: number) => {
	const next = text[index];
	if (next === undefined || /[\s,;:!?)\]}"'>#]/.test(next)) {
		return true;
	}
	return (
		next === "." && (index + 1 === text.length || /\s/.test(text[index + 1]))
	);
};

// The next word continues the path when it holds a slash and does not
// start a rooted path of its own, as "docs/missing/" does after "/repo/my"
// in "/repo/my docs/missing/". An entry ending there is only a prefix.
const continuesPath = (text: string, index: number) => {
	const word = /^\s+(\S+)/.exec(text.slice(index))?.[1];
	return Boolean(word?.includes("/") && !/^[/~]/.test(word));
};

const isEntryEnd = (text: string, index: number) =>
	isPathEnd(text, index) && !continuesPath(text, index);

const closingQuotes: Record<string, string> = {
	'"': '"',
	"'": "'",
	"“": "”",
	"‘": "’",
};

/**
 * Where a path that `opener` quotes ends in `text`, the text that follows
 * the opening quote, or -1 when `opener` is not a quote or does not close.
 * A verified path must end exactly there, so a prefix of the quoted path
 * never links.
 */
export const findQuotedPathEnd = (text: string, opener: string | undefined) => {
	const closer = opener === undefined ? undefined : closingQuotes[opener];
	if (!closer) {
		return -1;
	}
	const lineEnd = text.indexOf("\n");
	const end = text.indexOf(closer);
	return lineEnd !== -1 && end > lineEnd ? -1 : end;
};

const lineSuffixRegExp = /^(?::(\d+)(?::\d+)?|#L(\d+))/;

/**
 * A directory's entries, `null` when listing it failed, or `undefined`
 * while the listing is not available yet.
 */
type DirectoryEntries = readonly DirectoryEntry[] | null | undefined;

type FilePathWalk = {
	/** The directories the walk has needed so far, in order. */
	directories: string[];
	/** The verified file; `null` when none matches, `undefined` while pending. */
	verified: VerifiedFilePath | null | undefined;
};

/**
 * Walks directory listings to find the longest path that `text` starts
 * with, beginning in `directory`. At each level it takes the longest entry
 * name the remaining text starts with, followed by `/` for a directory or a
 * word boundary. A directory followed by a word boundary, with or without a
 * trailing `/`, ends the walk as a verified folder. A file or directory
 * whose name is followed by more path, as in "my docs/missing/" after
 * "my", does not fit. `.` and `..` segments move between directories the
 * walk has listed; `..` above `directory` matches nothing. The walk stops
 * with no match when a listing failed or the walk exceeds its depth, and
 * stays pending at the first directory whose listing is not available.
 */
export const walkFilePath = (
	directory: string,
	text: string,
	getEntries: (directory: string) => DirectoryEntries,
): FilePathWalk => {
	const directories: string[] = [];
	const parents: string[] = [];
	let current = directory;
	let offset = 0;
	for (let depth = 0; depth < maxWalkDepth; depth++) {
		for (;;) {
			if (text.startsWith("./", offset)) {
				offset += 2;
			} else if (text.startsWith("../", offset)) {
				const parent = parents.pop();
				if (parent === undefined) {
					return { directories, verified: null };
				}
				current = parent;
				offset += 3;
			} else {
				break;
			}
		}
		if (!directories.includes(current)) {
			directories.push(current);
		}
		const entries = getEntries(current);
		if (entries === undefined) {
			return { directories, verified: undefined };
		}
		if (entries === null) {
			return { directories, verified: null };
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
			const fits =
				(entry.is_dir && rest[length] === "/") || isEntryEnd(rest, length);
			if (fits) {
				best = entry;
			}
		}
		if (!best) {
			return { directories, verified: null };
		}
		if (best.is_dir) {
			const slash = rest[best.name.length] === "/";
			const end = offset + best.name.length + (slash ? 1 : 0);
			if (!slash || isPathEnd(text, end)) {
				return {
					directories,
					verified: {
						absolutePath: best.absolute_path_string,
						length: end,
						isDirectory: true,
					},
				};
			}
			parents.push(current);
			current = best.absolute_path_string;
			offset = end;
			continue;
		}
		const end = offset + best.name.length;
		const suffix = lineSuffixRegExp.exec(text.slice(end));
		const line = Number(suffix?.[1] ?? suffix?.[2]);
		const verified =
			suffix && line > 0 && isPathEnd(text, end + suffix[0].length)
				? {
						absolutePath: best.absolute_path_string,
						length: end + suffix[0].length,
						line,
					}
				: { absolutePath: best.absolute_path_string, length: end };
		return { directories, verified };
	}
	return { directories, verified: null };
};
