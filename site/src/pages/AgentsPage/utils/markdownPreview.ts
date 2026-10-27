import type { Nodes as MdastNodes } from "mdast";
import remarkParse from "remark-parse";
import { unified } from "unified";

type HastNode = {
	type: string;
	tagName?: string;
	value?: string;
	properties?: Record<string, unknown>;
	children?: HastNode[];
};

/** Prefix for ids and names from previewed markdown, as GitHub uses. */
export const previewIdPrefix = "user-content-";

/** At most this many workspace images load in one preview. */
export const maxPreviewImages = 20;

// A block that may hold a link reference definition; only those are parsed.
const definitionLineRegExp = /^ {0,3}(?:> ?)*\[[^\]\n]+\]:/m;

const markdownParser = unified().use(remarkParse);

const findDefinitions = (node: MdastNodes, found: string[]) => {
	if (node.type === "definition") {
		// Rebuilt from the parsed fields so a definition that spans lines, or
		// sits in a block quote, still resolves at the top level of a chunk.
		const title =
			node.title === null || node.title === undefined
				? ""
				: ` "${node.title.replace(/[\\"]/g, "\\$&")}"`;
		found.push(`[${node.label ?? node.identifier}]: <${node.url}>${title}`);
		return;
	}
	if ("children" in node) {
		for (const child of node.children) {
			findDefinitions(child, found);
		}
	}
};

/**
 * Groups markdown blocks into chunks of about `chunkSize` characters so a
 * large file renders and parses a piece at a time. Parsing one document
 * is superlinear in its size (GFM tables), and one render blocks the page
 * until all of it is done. The file's link reference definitions, taken
 * from the parsed blocks so ones in code or comments do not count, lead
 * every chunk: CommonMark uses the first definition of a label, so each
 * chunk resolves references the way the whole file does.
 */
export const groupMarkdownBlocks = (
	blocks: readonly string[],
	chunkSize: number,
): string[] => {
	const chunks: string[] = [];
	const definitions: string[] = [];
	let current = "";
	for (const block of blocks) {
		if (current && current.length + block.length > chunkSize) {
			chunks.push(current);
			current = "";
		}
		current += block;
		if (definitionLineRegExp.test(block)) {
			findDefinitions(markdownParser.parse(block), definitions);
		}
	}
	if (current) {
		chunks.push(current);
	}
	if (definitions.length === 0 || chunks.length < 2) {
		return chunks;
	}
	const prefix = `${definitions.join("\n")}\n\n`;
	return chunks.map((chunk) => prefix + chunk);
};

/**
 * Counts the images a chunk might hold, an upper bound on its workspace
 * images, so the image cap holds across chunks.
 */
export const countImageCandidates = (markdown: string) =>
	markdown.match(/!\[|<img\b/gi)?.length ?? 0;

const schemeRegExp = /^[a-z][a-z0-9+.-]*:/i;

/** True for URLs that leave the workspace, such as https: or mailto:. */
const isExternalUrl = (url: string) =>
	schemeRegExp.test(url) || url.startsWith("//");

/** True for an image source that names a workspace file. */
export const isWorkspaceImageSource = (src: string) =>
	src !== "" && !isExternalUrl(src) && !src.startsWith("#");

const normalizePath = (path: string) => {
	const segments: string[] = [];
	for (const segment of path.split("/")) {
		if (segment === "" || segment === ".") {
			continue;
		}
		if (segment === "..") {
			segments.pop();
		} else {
			segments.push(segment);
		}
	}
	return `/${segments.join("/")}`;
};

const decode = (value: string) => {
	try {
		return decodeURIComponent(value);
	} catch {
		return value;
	}
};

/**
 * Resolves a workspace-relative URL from a previewed file. Paths resolve
 * against the file's directory; root-relative paths resolve against the
 * repository root, as on GitHub, or the filesystem root outside one.
 */
export const resolveWorkspaceUrl = (
	url: string,
	directory: string,
	repoRoot: string | undefined,
): string => {
	const path = decode(url.replace(/[?#].*$/, ""));
	if (path.startsWith("/")) {
		return normalizePath(`${repoRoot ?? ""}${path}`);
	}
	return normalizePath(`${directory}/${path}`);
};

type PreviewLink =
	| { kind: "anchor"; id: string }
	| { kind: "external" }
	| { kind: "file"; path: string; line?: number };

/** Classifies a link in a previewed markdown file. */
export const resolvePreviewLink = (
	href: string,
	directory: string,
	repoRoot: string | undefined,
): PreviewLink => {
	if (isExternalUrl(href)) {
		return { kind: "external" };
	}
	const hash = href.indexOf("#");
	const fragment = hash === -1 ? "" : href.slice(hash + 1);
	const target = hash === -1 ? href : href.slice(0, hash);
	if (target.replace(/\?.*$/, "") === "") {
		return { kind: "anchor", id: decode(fragment) };
	}
	const path = resolveWorkspaceUrl(target, directory, repoRoot);
	const line = Number(/^L(\d+)/.exec(fragment)?.[1]);
	return line > 0 ? { kind: "file", path, line } : { kind: "file", path };
};

/** Concatenates the text inside a hast node. */
export const getHastText = (node: HastNode): string => {
	if (node.type === "text") {
		return node.value ?? "";
	}
	return (node.children ?? []).map(getHastText).join("");
};

// GitHub's heading anchors: lowercase, punctuation removed, spaces to
// hyphens. Repeats get a numeric suffix.
const slugBase = (text: string) =>
	text
		.trim()
		.toLowerCase()
		.replace(/[^\p{L}\p{M}\p{N}\p{Pc} -]/gu, "")
		.replace(/ /g, "-");

const removeStyleElements = (node: HastNode) => {
	if (!node.children) {
		return;
	}
	node.children = node.children.filter(
		(child) => child.type !== "element" || child.tagName !== "style",
	);
	for (const child of node.children) {
		removeStyleElements(child);
	}
};

/**
 * Rehype plugin, run before sanitizing, that removes `<style>` elements.
 * Sanitizing alone unwraps them and leaves their CSS as visible text.
 */
export const rehypeRemoveStyleElements = () => (tree: HastNode) =>
	removeStyleElements(tree);

type PreviewState = {
	slugs: Map<string, number>;
	bases: string[];
	images: number;
};

const markPreviewNode = (node: HastNode, state: PreviewState) => {
	if (node.type === "element") {
		node.properties ??= {};
		const { properties } = node;
		for (const key of ["id", "name"]) {
			const value = properties[key];
			if (typeof value === "string" && value !== "") {
				properties[key] = `${previewIdPrefix}${value}`;
			}
		}
		if (/^h[1-6]$/.test(node.tagName ?? "") && properties.id === undefined) {
			const base = slugBase(getHastText(node));
			const count = state.slugs.get(base) ?? 0;
			state.slugs.set(base, count + 1);
			state.bases.push(base);
			properties.id = `${previewIdPrefix}${count === 0 ? base : `${base}-${count}`}`;
		}
		const src = properties.src;
		if (
			node.tagName === "img" &&
			typeof src === "string" &&
			isWorkspaceImageSource(src)
		) {
			properties.dataPreviewImage = state.images++;
		}
	}
	for (const child of node.children ?? []) {
		markPreviewNode(child, state);
	}
};

/** Heading slug bases of a preview's chunks, by chunk index. */
export type PreviewHeadings = Map<number, readonly string[]>;

type FilePreviewOptions = {
	/** Number of the chunk's first workspace image. */
	firstImage?: number;
	/** Index of the chunk in the file. */
	chunk?: number;
	/**
	 * Slug bases of the file's chunks. Earlier chunks render first, so a
	 * repeated heading gets the suffix it has in the whole file; the chunk
	 * records its own bases for the chunks after it.
	 */
	headings?: PreviewHeadings;
	/** Distinguishes the plugin options of one preview from another's. */
	preview?: string;
};

/**
 * Rehype plugin, run after sanitizing, that prepares a previewed file:
 * prefixes ids and names so they cannot clobber document properties, gives
 * headings GitHub-style anchor ids, and numbers workspace images so the
 * preview can cap how many it loads.
 */
export const rehypeFilePreview =
	({ firstImage = 0, chunk = 0, headings }: FilePreviewOptions = {}) =>
	(tree: HastNode) => {
		const slugs = new Map<string, number>();
		for (const [index, bases] of headings ?? []) {
			if (index >= chunk) {
				continue;
			}
			for (const base of bases) {
				slugs.set(base, (slugs.get(base) ?? 0) + 1);
			}
		}
		const state = { slugs, bases: [], images: firstImage };
		markPreviewNode(tree, state);
		headings?.set(chunk, state.bases);
	};
