/*
 * Pure string transforms used by sync-docs.mjs: no filesystem or network access
 * and no top-level side effects, so transform.test.mjs can pin them. See
 * README.md for the fence-scanner design, the .mdx-forward escapes, and the
 * frontmatter/title and link-rewrite contracts.
 */
import { posix } from "node:path";
import { frontmatter as parseYamlFrontmatter } from "fumadocs-core/content/md/frontmatter";

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

// Track fenced-code state one line at a time. `state` is { inFence, marker,
// markerLen }. Low-level primitive; stepFence wraps it with blockquote
// awareness. See README.md ("One fence- and blockquote-aware line scanner").
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

// Advance blockquote-aware fence state by one line and classify it. `plain`
// tracks a top-level fence, `quoted` a fence nested in a blockquote that
// fenceScan alone misses. Returns updated plain/quoted plus a classifier
// { fenced, delim, opening, quotedDelim, match }; a blockquoted delimiter's
// match omits the `>` prefix, so callers must not rebuild the line from it.
function stepFence(line, plain, quoted) {
	if (!plain.inFence) {
		const quote = /^(\s*>)+\s?/.exec(line);
		if (quote) {
			const scan = fenceScan(line.slice(quote[0].length), quoted);
			const delim = !!scan.match;
			return {
				plain,
				quoted: scan.next,
				fenced: delim || scan.next.inFence,
				delim,
				opening: scan.opening,
				quotedDelim: delim,
				match: scan.match,
			};
		}
		if (quoted.inFence) quoted = { inFence: false, marker: "", markerLen: 0 };
	}
	const scan = fenceScan(line, plain);
	const delim = !!scan.match;
	return {
		plain: scan.next,
		quoted,
		fenced: delim || scan.next.inFence,
		delim,
		opening: scan.opening,
		quotedDelim: false,
		match: scan.match,
	};
}

// Walk the lines of `content` with blockquote-aware fence tracking, calling
// `fn(line, info)` and using its return as the replacement. `info` is
// { index, prose, delim, opening, quoted, match }, where `prose` means outside
// any fence and safe to transform. The single line walker; routing every
// transform through it keeps prose passes out of fenced content. See README.md.
function mapLines(content, fn) {
	const lines = content.split("\n");
	let plain = { inFence: false, marker: "", markerLen: 0 };
	let quoted = { inFence: false, marker: "", markerLen: 0 };
	for (let i = 0; i < lines.length; i++) {
		const s = stepFence(lines[i], plain, quoted);
		plain = s.plain;
		quoted = s.quoted;
		lines[i] = fn(lines[i], {
			index: i,
			prose: !s.fenced,
			delim: s.delim,
			opening: s.opening,
			quoted: s.quotedDelim,
			match: s.match,
		});
	}
	return lines.join("\n");
}

// Apply `cb` to the prose lines of `content`, leaving fence delimiters and
// fenced content (including blockquoted fences) untouched.
function mapProseLines(content, cb) {
	return mapLines(content, (line, info) => (info.prose ? cb(line) : line));
}

// Normalize opening code-fence language labels: map known aliases and
// lowercase everything so a strict highlighter (Shiki) sees a consistent set.
// Closing fences, content lines, and blockquoted fences are left untouched (a
// blockquoted delimiter's match omits the `>` prefix, so it is not rebuilt).
export function normalizeFences(content) {
	return mapLines(content, (line, info) => {
		if (info.delim && info.opening && !info.quoted && info.match[4]) {
			const m = info.match;
			const lower = m[4].toLowerCase();
			return `${m[1]}${m[2]}${LANG_ALIAS[lower] || lower}${m[5]}`;
		}
		return line;
	});
}

// Rewrite Coder's `## Step N: Title` headings into Fumadocs' native step form
// `## Title [step]` (skipping fences), so remarkSteps numbers them and the slug
// becomes just `Title`. Matches h1..h6, `Step` case-insensitive.
export function normalizeStepHeadings(content) {
	const stepHeading = /^(#{1,6})[ \t]+Step[ \t]+\d+:[ \t]*(.*?)[ \t]*$/i;
	return mapLines(content, (line, info) => {
		if (!info.prose) return line;
		const m = stepHeading.exec(line);
		return m ? `${m[1]} ${m[2]} [step]` : line;
	});
}

// Strip HTML comments (`<!-- ... -->`) outside fenced code and inline code,
// including multi-line ones, preserving line count so block structure is
// unchanged. Runs before every other transform. Returns
// `{ content, unclosedCommentLine }`, the 1-based line of a comment that opens
// but never closes (else null) so the caller can fail the sync and name the
// file rather than silently swallow the rest. See README.md ("HTML comments").
export function stripHtmlComments(content) {
	const lines = content.split("\n");
	let plain = { inFence: false, marker: "", markerLen: 0 };
	let quoted = { inFence: false, marker: "", markerLen: 0 };
	let inComment = false;
	let commentOpenLine = -1;
	for (let i = 0; i < lines.length; i++) {
		// While inside a multi-line comment the body is opaque: do not advance
		// fence state (a ``` inside a comment is comment text, not a fence).
		if (!inComment) {
			const s = stepFence(lines[i], plain, quoted);
			plain = s.plain;
			quoted = s.quoted;
			if (s.fenced) continue; // delimiter or fenced content: leave untouched
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
				continue;
			}
			// Outside a comment, an inline code span is opaque: a `<!--` inside
			// backticks is code, not a comment opener. Whichever comes first, the
			// next code span or the next opener, is handled first.
			const tick = line.indexOf("`", pos);
			const start = line.indexOf("<!--", pos);
			if (start === -1 && tick === -1) {
				result += line.slice(pos);
				pos = line.length;
			} else if (tick !== -1 && (start === -1 || tick < start)) {
				result += line.slice(pos, tick);
				const spanLen = inlineCodeSpanLen(line, tick);
				result += line.slice(tick, tick + spanLen);
				pos = tick + spanLen;
			} else {
				result += line.slice(pos, start);
				pos = start + 4;
				inComment = true;
				commentOpenLine = i + 1;
			}
		}
		if (result !== line) lines[i] = result.trimEnd();
	}
	return {
		content: lines.join("\n"),
		unclosedCommentLine: inComment ? commentOpenLine : null,
	};
}

// Length of the opaque inline-code run at `line[start]`: the whole span when
// closed by a later run of the same length, else just the run (literal
// backticks). CommonMark code-span rule; per-line, so spans crossing line
// boundaries are not handled (no corpus multi-line span contains `<!--`).
function inlineCodeSpanLen(line, start) {
	let n = 0;
	while (line[start + n] === "`") n++;
	let k = start + n;
	while (k < line.length) {
		const idx = line.indexOf("`".repeat(n), k);
		if (idx === -1) break;
		let m = 0;
		while (line[idx + m] === "`") m++;
		if (m === n) return idx + n - start;
		k = idx + m;
	}
	return n;
}

// Apply `fn` to the parts of `line` outside inline code spans, leaving code
// spans byte-identical. The single inline-code scanner every prose transform
// shares (link rewriting, comment stripping, brace/autolink/void
// normalization), so a `.md` link, HTML comment, or `{token}` inside backticks
// is never rewritten.
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
		const spanLen = inlineCodeSpanLen(line, i);
		out += line.slice(i, i + spanLen);
		i += spanLen;
	}
	return out;
}

// Markdown autolinks (`<https://x>`, `<user@host>`). The negative lookbehind
// skips a CommonMark angle-bracket link *destination* (`[text](<url>)`, and the
// image form `![alt](<url>)`): those angle brackets delimit a URL that may
// contain parentheses and must be left intact, not rewritten into a nested
// `[url](url)` that ships a broken link. `\s*` covers the legal `]( <url> )`
// spacing; a plain parenthetical autolink `(<https://x>)` has no `]` and is
// still rewritten.
const URL_AUTOLINK = /(?<!\]\(\s*)<(https?:\/\/[^\s<>]+)>/g;
const EMAIL_AUTOLINK =
	/(?<!\]\(\s*)<([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,})>/g;

// Forward-looking .mdx escape, no-op at render in the current .md pipeline (see
// README.md). Rewrite autolinks to explicit links and backslash-escape a stray
// `<`, but leave `<` before a letter or `/` alone so real HTML tags stay tags.
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

// Forward-looking .mdx escape, no-op at render in the current .md pipeline where
// `{` is literal (see README.md). Escape literal braces in prose so a future
// .mdx flip does not read `{session_id}` as a JS expression.
export function escapeCurlyBraces(content) {
	return mapProseLines(content, (line) =>
		mapOutsideInlineCode(line, (seg) => seg.replace(/(?<!\\)([{}])/g, "\\$1")),
	);
}

// Forward-looking .mdx normalization, no-op at render in the current .md
// pipeline (see README.md). Self-close any not-already-closed void element so
// MDX does not error on a bare `<br>`; `/>` tags are left byte-identical and the
// full void-element set is covered.
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

// Thrown by parseFrontmatter when a file's leading `---` block is present but is
// not valid YAML. Carries the js-yaml `reason` and a stable `name` so callers
// can catch it (rather than the raw third-party YAMLException) and attribute the
// failure to the source file being parsed instead of crashing on an opaque
// parser stack trace.
export class FrontmatterError extends Error {
	constructor(reason) {
		super(`invalid YAML frontmatter: ${reason}`);
		this.name = "FrontmatterError";
		this.reason = reason;
	}
}

// Parse a leading YAML frontmatter block with the YAML helper (so quoting and
// escapes decode), counting only a mapping body as frontmatter so a `---`
// thematic break stays content. Returns `{ title, description, endLine }`:
// non-empty string values or null, and `endLine`, the first body line (0 when
// there is no frontmatter). See README.md ("Frontmatter and title") for the
// make gen H1 collision this avoids.
export function parseFrontmatter(content) {
	const none = { title: null, description: null, endLine: 0 };
	let matter;
	let data;
	try {
		({ matter, data } = parseYamlFrontmatter(content));
	} catch (err) {
		// Convert js-yaml's file-less YAMLException into a FrontmatterError (see
		// its definition above) so the sync can name the offending file.
		throw new FrontmatterError(err?.reason ?? err?.message ?? String(err));
	}
	if (
		!matter ||
		data === null ||
		typeof data !== "object" ||
		Array.isArray(data)
	) {
		return none;
	}
	const pick = (v) => (typeof v === "string" && v.trim() !== "" ? v : null);
	// `matter` spans the block including both `---` delimiters and (when the file
	// has a body) the trailing newline. endLine is the first body line in the
	// original line coordinates: the number of lines `matter` occupies, minus one
	// when it ends in a newline (the common case) so the empty split tail is not
	// counted as a body line.
	const lineCount = matter.split("\n").length;
	const endLine = matter.endsWith("\n") ? lineCount - 1 : lineCount;
	return {
		title: pick(data.title),
		description: pick(data.description),
		endLine,
	};
}

// Resolve the page title by precedence (frontmatter `title`, first body H1
// outside fences, manifest title, title-cased route segment) and locate the
// lines to strip. Returns `{ title, h1Line, frontmatterEnd, description }` in
// the original line coordinates (h1Line -1 when absent). See README.md.
export function extractTitle(content, route, manifestMeta = new Map()) {
	const fm = parseFrontmatter(content);
	const body =
		fm.endLine > 0 ? content.split("\n").slice(fm.endLine).join("\n") : content;
	let h1Title = null;
	let h1BodyLine = -1;
	mapLines(body, (line, info) => {
		if (h1Title === null && info.prose) {
			const m = /^#\s+(.+?)\s*#*\s*$/.exec(line);
			if (m) {
				h1Title = m[1].replace(/`/g, "").trim();
				h1BodyLine = info.index;
			}
		}
		return line;
	});
	const h1Line = h1BodyLine === -1 ? -1 : fm.endLine + h1BodyLine;
	const meta = manifestMeta.get(route);
	const title =
		fm.title ?? h1Title ?? meta?.title ?? titleCase(lastSeg(route) || "Docs");
	return {
		title,
		h1Line,
		frontmatterEnd: fm.endLine,
		description: fm.description,
	};
}

// Rewrite a single link/image target relative to `currentRel` (relative to
// docs/), returning the new target or null to leave it unchanged. The `ctx`
// object supplies the environment (imageRemote, resolveMd, copyImage,
// sourceLink). See README.md ("Link rewriting").
export function rewriteTarget(target, currentRel, ctx) {
	if (/^(https?:|mailto:|tel:|#|\/\/)/i.test(target)) return null;
	const [rawPath, anchor] = target.split("#");
	if (!rawPath) return null;
	const withAnchor = (href) => (anchor ? `${href}#${anchor}` : href);

	let resolvedRel;
	if (rawPath.startsWith("/")) {
		resolvedRel = posix.normalize(rawPath.replace(/^\/+/, ""));
	} else {
		resolvedRel = posix.normalize(
			posix.join(posix.dirname(currentRel), rawPath),
		);
	}

	const lower = resolvedRel.toLowerCase();
	const ext = lower.slice(lower.lastIndexOf("."));

	// The target escapes the docs/ corpus (e.g. a source-tree link like
	// ../../coderd). It can never resolve to a synced page or a bundled image,
	// so rather than leave a relative path that 404s in the offline bundle,
	// point non-image targets at the file/dir on GitHub. Images that escape
	// docs/ are not rewritten (they would need bundling to work offline).
	if (resolvedRel.startsWith("..")) {
		if (IMAGE_EXT.has(ext) || !ctx.sourceLink) return null;
		const repoRel = posix.normalize(
			posix.join("docs", posix.dirname(currentRel), rawPath),
		);
		if (repoRel.startsWith("..")) return null;
		const href = ctx.sourceLink(repoRel);
		return href ? withAnchor(href) : null;
	}

	if (IMAGE_EXT.has(ext)) {
		if (ctx.imageRemote) return `${ctx.imageRemote}/${resolvedRel}`;
		return ctx.copyImage(resolvedRel);
	}
	if (lower.endsWith(".md")) {
		const href = ctx.resolveMd(resolvedRel);
		if (href) return withAnchor(href);
		return null;
	}
	return null;
}

function rewriteLine(line, currentRel, ctx) {
	// Route both replacers through the shared inline-code scanner so a link or
	// src/href inside `...` (e.g. a Markdown example) is left verbatim rather
	// than rewritten to a route (which would also trigger an image copy).
	return mapOutsideInlineCode(line, (seg) => {
		seg = seg.replace(
			/(\]\()([^)\s]+)(\s+"[^"]*")?(\))/g,
			(full, open, target, title, close) => {
				const next = rewriteTarget(target, currentRel, ctx);
				return next ? `${open}${next}${title || ""}${close}` : full;
			},
		);
		seg = seg.replace(
			/\b(src|href)=("|')([^"']+)\2/g,
			(full, attr, q, target) => {
				const next = rewriteTarget(target, currentRel, ctx);
				return next ? `${attr}=${q}${next}${q}` : full;
			},
		);
		return seg;
	});
}

// Rewrite Markdown links and inline HTML src/href attributes in `content`.
// Fenced code blocks (including blockquoted fences) and inline code spans are
// skipped so link-like text inside examples is left verbatim.
export function rewriteContent(content, currentRel, ctx) {
	return mapLines(content, (line, info) =>
		info.prose ? rewriteLine(line, currentRel, ctx) : line,
	);
}
