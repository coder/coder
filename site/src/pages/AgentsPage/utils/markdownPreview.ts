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

const linkDefinitionRegExp = /^ {0,3}\[[^\]\n]+\]:[ \t]*\S.*$/gm;

/**
 * Groups markdown blocks into chunks of about `chunkSize` characters so a
 * large file renders and parses a piece at a time. Parsing one document
 * is superlinear in its size (GFM tables), and one render blocks the page
 * until all of it is done. Link reference definitions are repeated in
 * every chunk so references resolve across chunk boundaries.
 */
export const groupMarkdownBlocks = (
	content: string,
	blocks: readonly string[],
	chunkSize: number,
): string[] => {
	const definitions = content.match(linkDefinitionRegExp)?.join("\n") ?? "";
	const chunks: string[] = [];
	let current = "";
	for (const block of blocks) {
		if (current && current.length + block.length > chunkSize) {
			chunks.push(current);
			current = "";
		}
		current += block;
	}
	if (current) {
		chunks.push(current);
	}
	if (!definitions || chunks.length < 2) {
		return chunks;
	}
	return chunks.map((chunk) => `${chunk}\n\n${definitions}\n`);
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
// hyphens, and a numeric suffix for repeats.
const slugify = (text: string, seen: Map<string, number>) => {
	const base = text
		.trim()
		.toLowerCase()
		.replace(/[^\p{L}\p{M}\p{N}\p{Pc} -]/gu, "")
		.replace(/ /g, "-");
	const count = seen.get(base) ?? 0;
	seen.set(base, count + 1);
	return count === 0 ? base : `${base}-${count}`;
};

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
			properties.id = `${previewIdPrefix}${slugify(getHastText(node), state.slugs)}`;
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

/**
 * Rehype plugin, run after sanitizing, that prepares a previewed file:
 * prefixes ids and names so they cannot clobber document properties, gives
 * headings GitHub-style anchor ids, and numbers workspace images, starting
 * at `firstImage`, so the preview can cap how many it loads.
 */
export const rehypeFilePreview =
	({ firstImage = 0 }: { firstImage?: number } = {}) =>
	(tree: HastNode) =>
		markPreviewNode(tree, { slugs: new Map(), images: firstImage });
