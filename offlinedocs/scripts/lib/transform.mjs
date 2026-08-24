// Pure string transforms for the docs sync: fence-aware line scanning,
// frontmatter/title extraction, and link rewriting. No I/O, no side effects.
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

// Track fenced-code state one line at a time; stepFence adds blockquote awareness.
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
	// CommonMark close: same marker char, run at least as long as the opener, no info string.
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

// Advance fence state one line, tracking top-level (plain) and blockquoted (quoted)
// fences. A blockquoted delimiter's match omits the ">" prefix; do not rebuild the
// line from it.
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

// Walk lines with fence tracking, replacing each with fn(line, info); info.prose is
// true outside any fence. Every prose transform routes through this to skip fences.
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

// Lowercase and alias opening code-fence language labels for a strict highlighter.
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

// Rewrite Coder "## Step N: Title" headings into Fumadocs' "## Title [step]" form.
export function normalizeStepHeadings(content) {
	const stepHeading = /^(#{1,6})[ \t]+Step[ \t]+\d+:[ \t]*(.*?)[ \t]*$/i;
	return mapLines(content, (line, info) => {
		if (!info.prose) return line;
		const m = stepHeading.exec(line);
		return m ? `${m[1]} ${m[2]} [step]` : line;
	});
}

// Strip HTML comments outside fenced and inline code, preserving line count.
// Returns { content, unclosedCommentLine }: the 1-based line of an unclosed
// comment (else null) so the caller can fail the sync and name the file.
export function stripHtmlComments(content) {
	const lines = content.split("\n");
	let plain = { inFence: false, marker: "", markerLen: 0 };
	let quoted = { inFence: false, marker: "", markerLen: 0 };
	let inComment = false;
	let commentOpenLine = -1;
	for (let i = 0; i < lines.length; i++) {
		// Inside a multi-line comment the body is opaque: do not advance fence state.
		if (!inComment) {
			const s = stepFence(lines[i], plain, quoted);
			plain = s.plain;
			quoted = s.quoted;
			if (s.fenced) continue;
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
			// Outside a comment an inline code span is opaque: "<!--" inside backticks
			// is not an opener. Handle whichever comes first, the span or the opener.
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

// Length of the inline-code run at line[start]: the whole span when closed by a
// later run of the same length, else just the opening run. Per-line (CommonMark).
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

// Apply fn to the parts of line outside inline code spans, leaving spans byte-identical.
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

// Autolinks (<https://x>, <user@host>). The lookbehind skips a CommonMark link
// destination [text](<url>) / ![alt](<url>) so its angle brackets stay intact.
const URL_AUTOLINK = /(?<!\]\(\s*)<(https?:\/\/[^\s<>]+)>/g;
const EMAIL_AUTOLINK =
	/(?<!\]\(\s*)<([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,})>/g;

// .mdx-forward escape, no-op in the current .md pipeline: rewrite autolinks and
// escape a stray "<", leaving "<" before a letter or "/" so real tags stay tags.
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

// .mdx-forward escape, no-op in the current .md pipeline: escape literal braces in prose.
export function escapeCurlyBraces(content) {
	return mapProseLines(content, (line) =>
		mapOutsideInlineCode(line, (seg) => seg.replace(/(?<!\\)([{}])/g, "\\$1")),
	);
}

// .mdx-forward normalization, no-op in the current .md pipeline: self-close void
// elements so a future .mdx flip does not error on a bare "<br>".
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

// Thrown by parseFrontmatter for an invalid leading YAML block, so callers can
// attribute the failure to the source file instead of a raw YAMLException.
export class FrontmatterError extends Error {
	constructor(reason) {
		super(`invalid YAML frontmatter: ${reason}`);
		this.name = "FrontmatterError";
		this.reason = reason;
	}
}

// Parse a leading YAML frontmatter block (mapping bodies only, so a "---" thematic
// break stays content). Returns { title, description, endLine }: string values or
// null, and endLine, the first body line (0 when there is no frontmatter).
export function parseFrontmatter(content) {
	const none = { title: null, description: null, endLine: 0 };
	let matter;
	let data;
	try {
		({ matter, data } = parseYamlFrontmatter(content));
	} catch (err) {
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
	// matter spans both "---" delimiters plus a trailing newline (when there is a
	// body); subtract that newline so the empty split tail is not a body line.
	const lineCount = matter.split("\n").length;
	const endLine = matter.endsWith("\n") ? lineCount - 1 : lineCount;
	return {
		title: pick(data.title),
		description: pick(data.description),
		endLine,
	};
}

// Resolve the page title by precedence (frontmatter, first body H1, manifest,
// route) and locate the lines to strip. Coordinates are original-line, h1Line -1
// when absent.
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

// Rewrite one link/image target relative to currentRel (a docs/-relative path),
// or return null to leave it unchanged. ctx supplies imageRemote/resolveMd/
// copyImage/sourceLink.
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

	// Target escapes docs/ (e.g. ../../coderd): it cannot resolve offline, so point
	// non-image targets at GitHub; escaping images are left alone (they need bundling).
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
	// Route both replacers through the inline-code scanner so a link inside "..." stays verbatim.
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

// Rewrite Markdown links and inline HTML src/href; skip fenced and inline code.
export function rewriteContent(content, currentRel, ctx) {
	return mapLines(content, (line, info) =>
		info.prose ? rewriteLine(line, currentRel, ctx) : line,
	);
}
