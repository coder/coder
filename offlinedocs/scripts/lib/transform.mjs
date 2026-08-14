/*
 * Pure, dependency-free string transforms used by sync-docs.mjs.
 *
 * These are kept in their own module (with no filesystem or network access and
 * no top-level side effects) so they can be unit-tested without running the
 * sync script. Environment specifics (the resolved image base, the file->route
 * map, image copying, source-tree links) are injected into the rewrite helpers
 * via a `ctx` object.
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
//
// This is the low-level, single-context primitive. Nothing outside this module
// calls it directly: stepFence wraps it with blockquote awareness, and mapLines
// and stripHtmlComments (the only two line walkers) both go through stepFence,
// so every transform observes fences the same way.
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

// Advance blockquote-aware fenced-code state by one line and classify the line.
// `plain` tracks a top-level fence; `quoted` tracks a fence nested inside a
// blockquote (`> ```...`), which `fenceScan` alone does not see. Returns the
// updated `plain`/`quoted` states plus:
//   fenced:  the line is a fence delimiter or sits inside a fenced block
//            (plain or blockquoted); prose transforms must skip it
//   delim:   the line is a fence delimiter (opening or closing)
//   opening: the delimiter opens a fence (only meaningful when delim is true)
//   quoted:  the delimiter is a blockquoted one (its `match` was taken against
//            the line with the `>` prefix stripped, so callers must not rebuild
//            the line from it)
//   match:   the fenceScan match for a delimiter line, else null
// This is the single fence scanner; mapLines and stripHtmlComments are its only
// callers, so every transform tracks fences (including blockquoted ones) the
// same way.
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
				quoted_delim: delim,
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
		quoted_delim: false,
		match: scan.match,
	};
}

// Walk the lines of `content` with blockquote-aware fence tracking, calling
// `fn(line, info)` for every line and using the return value as the
// replacement. `info` is:
//   { index, prose, delim, opening, quoted, match }
//   prose:   outside any fenced block and not a delimiter (safe to transform)
//   delim:   a fence delimiter line
//   opening: the delimiter opens a fence
//   quoted:  the delimiter is blockquoted (its match omits the `>` prefix)
//   match:   the fenceScan match for a delimiter line, else null
// Routing every line-level transform through this one walker is what keeps
// prose passes (link rewriting, brace escaping, autolink and void-element
// normalization) out of fenced content, including fences nested in blockquotes.
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
			quoted: s.quoted_delim,
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

// Rewrite Coder's `## Step N: Title` procedure headings into Fumadocs' native
// step form `## Title [step]`, skipping fenced code blocks. Fumadocs'
// `remarkSteps` detects the trailing ` [step]` marker, strips it, groups
// consecutive same-depth step headings into a numbered `.fd-steps` structure,
// and numbers them by position. Baking this into the synced source (rather
// than a build-time remark plugin) keeps the `.md` files themselves free of
// the redundant "Step N:" prefix, so the rendered title and its slug become
// just `Title`. Matches h1..h6 and treats `Step` case-insensitively.
export function normalizeStepHeadings(content) {
	const stepHeading = /^(#{1,6})[ \t]+Step[ \t]+\d+:[ \t]*(.*?)[ \t]*$/i;
	return mapLines(content, (line, info) => {
		if (!info.prose) return line;
		const m = stepHeading.exec(line);
		return m ? `${m[1]} ${m[2]} [step]` : line;
	});
}

// Strip HTML comments (`<!-- ... -->`) outside fenced code blocks, including
// comments spanning multiple lines. The rendered site shows nothing for an
// HTML comment either way (rehype-raw keeps it as an invisible comment node in
// the current `.md` pipeline), so dropping them only tidies the emitted source;
// it also keeps the corpus valid for a future `.mdx` flip, where an HTML
// comment is a parse error. The line count is preserved (a line that was only a
// comment becomes an empty line) so markdown block structure around comment
// lines, such as paragraph boundaries, is unchanged. Comments inside fenced
// code blocks (including blockquoted fences) are content and are left
// untouched. Runs before every other content transform so commented-out
// markdown never feeds them.
//
// Returns `{ content, unclosedCommentLine }`. A comment that opens but never
// closes would otherwise swallow the rest of the file to end-of-input silently;
// `unclosedCommentLine` is the 1-based line where such an unterminated comment
// began (else null) so the caller can fail the sync and name the file instead
// of shipping a truncated page.
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
			} else {
				const start = line.indexOf("<!--", pos);
				if (start === -1) {
					result += line.slice(pos);
					pos = line.length;
				} else {
					result += line.slice(pos, start);
					pos = start + 4;
					inComment = true;
					commentOpenLine = i + 1;
				}
			}
		}
		if (result !== line) lines[i] = result.trimEnd();
	}
	return {
		content: lines.join("\n"),
		unclosedCommentLine: inComment ? commentOpenLine : null,
	};
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

const URL_AUTOLINK = /<(https?:\/\/[^\s<>]+)>/g;
const EMAIL_AUTOLINK = /<([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,})>/g;

// Forward-looking escape for the planned `.mdx` flip; a no-op at render in the
// current `.md` + rehype-raw pipeline, which the rewrites are chosen to render
// identically. MDX removes Markdown autolinks (`<https://x>`, `<user@host>`)
// and treats a stray `<` before anything that cannot start a JSX tag as a parse
// error. Rewrite autolinks to explicit links (render-identical in .md and .mdx)
// and backslash-escape stray `<` (a valid escape in both, rendering a literal
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

// Forward-looking escape for the planned `.mdx` flip; a no-op at render in the
// current `.md` pipeline, where `{` is literal. Under MDX, `{` starts a JS
// expression: invalid contents fail the build, and contents that happen to
// parse as JS (`{session_id}`) are worse, silently evaluated and swallowed at
// render. Escaping literal braces in prose (`\{` and `\}` are valid escapes in
// both .md and .mdx, rendering the brace) renders identically today and
// unblocks that flip. Fenced blocks (including blockquoted fences) and inline
// code spans are left untouched.
export function escapeCurlyBraces(content) {
	return mapProseLines(content, (line) =>
		mapOutsideInlineCode(line, (seg) => seg.replace(/(?<!\\)([{}])/g, "\\$1")),
	);
}

// Forward-looking normalization for the planned `.mdx` flip; a no-op at render
// in the current `.md` + rehype-raw pipeline, where a void element is void with
// or without the trailing slash. MDX/JSX requires void HTML elements to be
// self-closed: a bare `<br>`, `<img ...>`, or `<source ...>` opens an element
// that never closes, so the MDX parser errors ("Expected a closing tag for
// `<br>`"). Rewrite any void element that is not already self-closed to its
// `<tag ... />` form, leaving tags that already end in `/>` byte-identical and
// preserving attributes verbatim. The full HTML void-element set is covered so
// the transform is complete rather than census-driven. Invalid closing forms
// such as `</br>` are intentionally left alone: those are fixed at the source
// in coder/coder. Fenced blocks (including blockquoted fences) and inline code
// spans are left untouched.
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
	let title = null;
	let h1Line = -1;
	mapLines(content, (line, info) => {
		if (title === null && info.prose) {
			const m = /^#\s+(.+?)\s*#*\s*$/.exec(line);
			if (m) {
				title = m[1].replace(/`/g, "").trim();
				h1Line = info.index;
			}
		}
		return line;
	});
	if (title !== null) return { title, h1Line };
	const meta = manifestMeta.get(route);
	if (meta?.title) return { title: meta.title, h1Line: -1 };
	return { title: titleCase(lastSeg(route) || "Docs"), h1Line: -1 };
}

// Rewrite a single link/image target relative to `currentRel` (a path relative
// to the docs/ root). Returns the new target, or null to leave it unchanged.
// `ctx` supplies the environment:
//   ctx.imageRemote        remote image base URL, or '' for local-copy mode
//   ctx.resolveMd(rel)     resolved `.md` path -> route href, or null if unmapped
//   ctx.copyImage(rel)     copy a local image and return its href, or null
//   ctx.sourceLink(repoRel) a source-tree target (a repo path outside docs/)
//                           -> a GitHub URL, or null if unsupported
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
// Fenced code blocks (including blockquoted fences) are skipped so link-like
// text inside examples is left verbatim.
export function rewriteContent(content, currentRel, ctx) {
	return mapLines(content, (line, info) =>
		info.prose ? rewriteLine(line, currentRel, ctx) : line,
	);
}
