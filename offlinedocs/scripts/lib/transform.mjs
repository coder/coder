/*
 * Pure, dependency-free string transforms used by sync-docs.mjs.
 *
 * These are kept in their own module (with no filesystem or network access and
 * no top-level side effects) so they can be unit-tested without running the
 * sync script. Environment specifics (the resolved image base, the file->route
 * map, image copying) are injected into the rewrite helpers via a `ctx` object.
 */
import { posix } from "node:path";

const IMAGE_EXT = new Set([
	".png",
	".svg",
	".jpg",
	".jpeg",
	".gif",
	".webp",
	".avif",
	".ico",
]);

const LANG_ALIAS = {
	env: "ini",
	dotenv: "ini",
	pwsh: "powershell",
};

export function slugSegment(seg) {
	const s = seg
		.toLowerCase()
		.replace(/\.md$/, "")
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
	return s || "page";
}

export function isIndexFile(base) {
	return /^(index|readme)\.md$/i.test(base);
}

export function normalizeManifestPath(p) {
	return (p || "").replace(/^\.\//, "").replace(/^\//, "");
}

export function titleCase(s) {
	return s.replace(/[-_]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function lastSeg(route) {
	const i = route.lastIndexOf("/");
	return i === -1 ? route : route.slice(i + 1);
}

// Track fenced-code-block state one line at a time so link rewriting, title
// extraction, and fence normalization all agree on where fences begin and end.
// `state` is { inFence, marker, markerLen }: the fence character (backtick or
// tilde) and the length of the run that opened the current block. Following
// CommonMark, a fence closes only on the same marker, a run at least as long as
// the opener, and no info string, so a ``` line cannot close a ~~~ block and a
// short ``` cannot close a longer ````` block.
function fenceScan(line, state) {
	const match = /^(\s*)(```+|~~~+)(\s*)([^\s`]*)(.*)$/.exec(line);
	if (!match) return { match: null, opening: false, next: state };
	if (!state.inFence) {
		return {
			match,
			opening: true,
			next: { inFence: true, marker: match[2][0], markerLen: match[2].length },
		};
	}
	// A closing fence uses the same marker character, a run at least as long as
	// the opener, and no info string. A different marker, a shorter run, or an
	// info string is content, so stay in the current block.
	if (
		match[2][0] === state.marker &&
		match[2].length >= state.markerLen &&
		!match[4]
	) {
		return {
			match,
			opening: false,
			next: { inFence: false, marker: "", markerLen: 0 },
		};
	}
	return { match, opening: false, next: state };
}

// Normalize opening code-fence language labels: map known aliases and
// lowercase everything so a strict highlighter (Shiki) sees a consistent set.
// Closing fences and content lines are left untouched.
export function normalizeFences(content) {
	const lines = content.split("\n");
	let state = { inFence: false, marker: "", markerLen: 0 };
	for (let i = 0; i < lines.length; i++) {
		const { match, opening, next } = fenceScan(lines[i], state);
		state = next;
		if (opening && match[4]) {
			const lower = match[4].toLowerCase();
			lines[i] =
				`${match[1]}${match[2]}${LANG_ALIAS[lower] || lower}${match[5]}`;
		}
	}
	return lines.join("\n");
}

// Rewrite Coder's `## Step N: Title` procedure headings into Fumadocs' native
// step form `## Title [step]`, skipping fenced code blocks. Fumadocs'
// `remarkSteps` detects the trailing ` [step]` marker, strips it, groups
// consecutive same-depth step headings into a numbered `.fd-steps` structure,
// and numbers them by position. Baking this into the synced source (rather
// than a build-time remark plugin) keeps the `.md` files themselves free of
// the redundant "Step N:" prefix, so the rendered title and its slug become
// just `Title`. Matches h1..h6 and treats `Step` case-insensitively.
export function normalizeStepHeadings(content) {
	const lines = content.split("\n");
	let state = { inFence: false, marker: "", markerLen: 0 };
	const stepHeading = /^(#{1,6})[ \t]+Step[ \t]+\d+:[ \t]*(.*?)[ \t]*$/i;
	for (let i = 0; i < lines.length; i++) {
		const { match, next } = fenceScan(lines[i], state);
		state = next;
		if (match) continue; // fence delimiter line
		if (state.inFence) continue; // inside a fenced block
		const m = stepHeading.exec(lines[i]);
		if (m) lines[i] = `${m[1]} ${m[2]} [step]`;
	}
	return lines.join("\n");
}

// Strip HTML comments (`<!-- ... -->`) outside fenced code blocks, including
// comments spanning multiple lines. MDX forbids HTML comments, and they carry
// no reader value in the rendered site, so the synced corpus drops them (the
// upstream coder/coder source keeps its comments for authors). The line count
// is preserved (a line that was only a comment becomes an empty line) so
// markdown block structure around comment lines, such as paragraph
// boundaries, is unchanged. Comments inside fenced code blocks are code and
// are left untouched. Runs before every other content transform so commented-
// out markdown never feeds them.
export function stripHtmlComments(content) {
	const lines = content.split("\n");
	let state = { inFence: false, marker: "", markerLen: 0 };
	let inComment = false;
	for (let i = 0; i < lines.length; i++) {
		if (!inComment) {
			const { match, next } = fenceScan(lines[i], state);
			state = next;
			if (match) continue; // fence delimiter line
			if (state.inFence) continue; // inside a fenced block
		}
		const line = lines[i];
		let result = "";
		let pos = 0;
		while (pos < line.length) {
			if (inComment) {
				const end = line.indexOf("-->", pos);
				if (end === -1) {
					pos = line.length;
				} else {
					pos = end + 3;
					inComment = false;
				}
			} else {
				const start = line.indexOf("<!--", pos);
				if (start === -1) {
					result += line.slice(pos);
					pos = line.length;
				} else {
					result += line.slice(pos, start);
					pos = start + 4;
					inComment = true;
				}
			}
		}
		if (result !== line) lines[i] = result.trimEnd();
	}
	return lines.join("\n");
}

// Apply `fn` to the parts of `line` outside inline code spans. CommonMark
// code spans open with a backtick run and close at the next run of the same
// length; unmatched runs are literal text. Spans crossing line boundaries are
// not handled (processing is per-line); the corpus has none.
function mapOutsideInlineCode(line, fn) {
	let out = "";
	let i = 0;
	while (i < line.length) {
		if (line[i] !== "`") {
			let j = line.indexOf("`", i);
			if (j === -1) j = line.length;
			out += fn(line.slice(i, j));
			i = j;
			continue;
		}
		let n = 0;
		while (line[i + n] === "`") n++;
		let close = -1;
		let k = i + n;
		while (k < line.length) {
			const idx = line.indexOf("`".repeat(n), k);
			if (idx === -1) break;
			let m = 0;
			while (line[idx + m] === "`") m++;
			if (m === n) {
				close = idx;
				break;
			}
			k = idx + m;
		}
		if (close === -1) {
			out += line.slice(i, i + n);
			i += n;
		} else {
			out += line.slice(i, close + n);
			i = close + n;
		}
	}
	return out;
}

// Iterate the prose lines of `content`, applying `cb` and using its return
// value as the replacement line. Skips fence delimiter lines and fenced
// content, including fences nested inside blockquotes (`> ```…`), which
// `fenceScan` alone does not see. A blockquote-nested fence is treated as
// closed when the blockquote context ends.
function mapProseLines(content, cb) {
	const lines = content.split("\n");
	let plain = { inFence: false, marker: "", markerLen: 0 };
	let quoted = { inFence: false, marker: "", markerLen: 0 };
	for (let i = 0; i < lines.length; i++) {
		if (!plain.inFence) {
			const quote = /^(\s*>)+\s?/.exec(lines[i]);
			if (quote) {
				const { match, next } = fenceScan(
					lines[i].slice(quote[0].length),
					quoted,
				);
				quoted = next;
				if (match || quoted.inFence) continue;
				lines[i] = cb(lines[i]);
				continue;
			}
			if (quoted.inFence) quoted = { inFence: false, marker: "", markerLen: 0 };
		}
		const { match, next } = fenceScan(lines[i], plain);
		plain = next;
		if (match) continue; // fence delimiter line
		if (plain.inFence) continue; // inside a fenced block
		lines[i] = cb(lines[i]);
	}
	return lines.join("\n");
}

const URL_AUTOLINK = /<(https?:\/\/[^\s<>]+)>/g;
const EMAIL_AUTOLINK = /<([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,})>/g;

// MDX removed Markdown autolinks (`<https://x>`, `<user@host>`) and treats a
// stray `<` before anything that cannot start a JSX tag as a parse error.
// Rewrite autolinks to explicit links (render-identical in .md and .mdx) and
// backslash-escape stray `<` (a valid escape in both, rendering a literal
// `<`). A `<` before a letter or `/` is left alone: real HTML tags must stay
// tags, and placeholder pseudo-tags such as `<your-coder-url>` are fixed
// upstream instead (browsers drop them even today). Fenced blocks and inline
// code spans are left untouched.
export function normalizeAngleBrackets(content) {
	return mapProseLines(content, (line) =>
		mapOutsideInlineCode(line, (seg) =>
			seg
				.replace(URL_AUTOLINK, "[$1]($1)")
				.replace(EMAIL_AUTOLINK, "[$1](mailto:$1)")
				.replace(/(^|[^\\])<(?![A-Za-z/])/g, "$1\\<"),
		),
	);
}

// MDX treats `{` as the start of a JS expression: invalid contents fail the
// build, and contents that happen to parse as JS (`{session_id}`) are worse,
// silently evaluated and swallowed at render. Escape literal braces in prose
// (`\{` and `\}` are valid escapes in both .md and .mdx, rendering the
// brace). Fenced blocks (including blockquoted fences) and inline code spans
// are left untouched.
export function escapeCurlyBraces(content) {
	return mapProseLines(content, (line) =>
		mapOutsideInlineCode(line, (seg) => seg.replace(/(?<!\\)([{}])/g, "\\$1")),
	);
}

// MDX/JSX requires void HTML elements to be self-closed. A bare `<br>`,
// `<img ...>`, or `<source ...>` opens an element that never closes, so the
// MDX parser errors ("Expected a closing tag for `<br>`"). Rewrite any void
// element that is not already self-closed to its `<tag ... />` form, leaving
// tags that already end in `/>` byte-identical and preserving attributes
// verbatim. The full HTML void-element set is covered so the transform is
// complete rather than census-driven. This renders identically in the current
// `.md` + rehype-raw pipeline (a void element is void with or without the
// slash) and unblocks the `.mdx` flip. Invalid closing forms such as `</br>`
// are intentionally left alone: those are fixed at the source in coder/coder.
// Fenced blocks (including blockquoted fences) and inline code spans are left
// untouched.
const VOID_ELEMENTS =
	"area|base|br|col|embed|hr|img|input|link|meta|param|source|track|wbr";
const VOID_TAG = new RegExp(`<(${VOID_ELEMENTS})\\b([^>]*?)\\s*(/?)>`, "gi");

export function selfCloseVoidElements(content) {
	return mapProseLines(content, (line) =>
		mapOutsideInlineCode(line, (seg) =>
			seg.replace(VOID_TAG, (m, tag, attrs, slash) =>
				slash ? m : `<${tag}${attrs} />`,
			),
		),
	);
}

// Extract the page title from the first H1 outside code fences, returning the
// line index of that H1 (so the caller can strip it). Falls back to the
// manifest title, then a title-cased route segment. `manifestMeta` maps a
// route to `{ title, description }`.
export function extractTitle(content, route, manifestMeta = new Map()) {
	const lines = content.split("\n");
	let state = { inFence: false, marker: "", markerLen: 0 };
	for (let i = 0; i < lines.length; i++) {
		const { match, next } = fenceScan(lines[i], state);
		state = next;
		if (match) continue; // fence delimiter line
		if (state.inFence) continue; // inside a fenced block
		const m = /^#\s+(.+?)\s*#*\s*$/.exec(lines[i]);
		if (m) {
			const title = m[1].replace(/`/g, "").trim();
			return { title, h1Line: i };
		}
	}
	const meta = manifestMeta.get(route);
	if (meta?.title) return { title: meta.title, h1Line: -1 };
	return { title: titleCase(lastSeg(route) || "Docs"), h1Line: -1 };
}

// Rewrite a single link/image target relative to `currentRel`. Returns the new
// target, or null to leave it unchanged. `ctx` supplies the environment:
//   ctx.imageRemote     remote image base URL, or '' for local-copy mode
//   ctx.resolveMd(rel)  resolved `.md` path -> route href, or null if unmapped
//   ctx.copyImage(rel)  copy a local image and return its href, or null
export function rewriteTarget(target, currentRel, ctx) {
	if (/^(https?:|mailto:|tel:|#|\/\/)/i.test(target)) return null;
	const [rawPath, anchor] = target.split("#");
	if (!rawPath) return null;
	let resolvedRel;
	if (rawPath.startsWith("/")) {
		resolvedRel = posix.normalize(rawPath.replace(/^\/+/, ""));
	} else {
		resolvedRel = posix.normalize(
			posix.join(posix.dirname(currentRel), rawPath),
		);
	}
	if (resolvedRel.startsWith("..")) return null;

	const lower = resolvedRel.toLowerCase();
	const ext = lower.slice(lower.lastIndexOf("."));

	if (IMAGE_EXT.has(ext)) {
		if (ctx.imageRemote) return `${ctx.imageRemote}/${resolvedRel}`;
		return ctx.copyImage(resolvedRel);
	}
	if (lower.endsWith(".md")) {
		const href = ctx.resolveMd(resolvedRel);
		if (href) return anchor ? `${href}#${anchor}` : href;
		return null;
	}
	return null;
}

function rewriteLine(line, currentRel, ctx) {
	line = line.replace(
		/(\]\()([^)\s]+)(\s+"[^"]*")?(\))/g,
		(full, open, target, title, close) => {
			const next = rewriteTarget(target, currentRel, ctx);
			return next ? `${open}${next}${title || ""}${close}` : full;
		},
	);
	line = line.replace(
		/\b(src|href)=("|')([^"']+)\2/g,
		(full, attr, q, target) => {
			const next = rewriteTarget(target, currentRel, ctx);
			return next ? `${attr}=${q}${next}${q}` : full;
		},
	);
	return line;
}

// Rewrite Markdown links and inline HTML src/href attributes in `content`.
// Fenced code blocks are skipped so link-like text inside examples is left
// verbatim.
export function rewriteContent(content, currentRel, ctx) {
	const lines = content.split("\n");
	let state = { inFence: false, marker: "", markerLen: 0 };
	for (let i = 0; i < lines.length; i++) {
		const { match, next } = fenceScan(lines[i], state);
		state = next;
		if (match) continue; // fence delimiter line
		if (state.inFence) continue; // inside a fenced block
		lines[i] = rewriteLine(lines[i], currentRel, ctx);
	}
	return lines.join("\n");
}
