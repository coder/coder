import fm from "front-matter";
import path from "path";

// Include directives let several pages share one Markdown fragment, usually
// kept under docs/_includes. This mirrors the hosted docs sync
// (coder/docs-engine, packages/docs-build/src/includes.ts) closely enough
// that the offline export renders the same text: a directive is a whole line
// holding exactly `<include>path</include>` outside fenced code, the path is
// relative to the file it is written in, the fragment's front matter is
// dropped, its lines take the directive's indentation, and its relative links
// and images are rewritten to resolve from the including page.

const INCLUDE_DIRECTIVE = /^([ \t]*)<include>([^<>]*)<\/include>[ \t]*$/;
const FENCE = /^[ \t]*(```|~~~)/;
const MAX_INCLUDE_DEPTH = 8;
const posix = path.posix;

const isRelative = (target: string) =>
	target !== "" && !/^([a-z][a-z0-9+.-]*:|#|\/)/i.test(target);

const rebase = (content: string, fragment: string, includer: string) => {
	const fromDir = posix.dirname(includer);
	const fragmentDir = posix.dirname(fragment);
	if (fromDir === fragmentDir) {
		return content;
	}
	const fix = (target: string) => {
		if (!isRelative(target)) {
			return target;
		}
		const [file, ...hash] = target.split("#");
		const resolved = posix.normalize(posix.join(fragmentDir, file));
		let relative = posix.relative(fromDir, resolved);
		if (!relative.startsWith(".")) {
			relative = `./${relative}`;
		}
		return hash.length > 0 ? `${relative}#${hash.join("#")}` : relative;
	};
	let inFence = false;
	return content
		.split("\n")
		.map((line) => {
			if (FENCE.test(line)) {
				inFence = !inFence;
				return line;
			}
			if (inFence) {
				return line;
			}
			return line
				.replace(
					/(\]\()([^)\s]+)/g,
					(_, open: string, target: string) => open + fix(target),
				)
				.replace(
					/(\ssrc=")([^"]+)/g,
					(_, open: string, target: string) => open + fix(target),
				);
		})
		.join("\n");
};

/**
 * Expands every include directive in one page, recursively.
 *
 * `page` is the page's path relative to docs/, and `read` returns the contents
 * of another file relative to docs/. A missing, unsafe, cyclic, or too deeply
 * nested include throws, so the export fails instead of shipping a page with
 * a hole in it.
 */
export const expandIncludes = (
	page: string,
	content: string,
	read: (relPath: string) => string,
	stack: string[] = [posix.normalize(page)],
): string => {
	if (!content.includes("<include>")) {
		return content;
	}
	const from = posix.normalize(page);
	const out: string[] = [];
	let inFence = false;
	for (const line of content.split("\n")) {
		if (FENCE.test(line)) {
			inFence = !inFence;
		}
		const match = inFence ? null : INCLUDE_DIRECTIVE.exec(line);
		if (!match) {
			out.push(line);
			continue;
		}
		const [, indent, target] = match;
		const fail = (reason: string): never => {
			throw new Error(`${from}: <include>${target}</include>: ${reason}`);
		};
		if (!isRelative(target) || target.includes("\\")) {
			fail("include paths must be relative to the including file");
		}
		const rel = posix.normalize(posix.join(posix.dirname(from), target));
		if (rel === ".." || rel.startsWith("../")) {
			fail("include path escapes docs/");
		}
		if (!rel.endsWith(".md")) {
			fail("only .md files can be included");
		}
		if (stack.includes(rel)) {
			fail(`include cycle: ${[...stack, rel].join(" -> ")}`);
		}
		if (stack.length > MAX_INCLUDE_DEPTH) {
			fail(`includes nested deeper than ${MAX_INCLUDE_DEPTH}`);
		}
		const { body } = fm(read(rel));
		const expanded = expandIncludes(rel, body, read, [...stack, rel]);
		const rebased = rebase(expanded, rel, from).replace(/\n+$/, "");
		for (const fragmentLine of rebased.split("\n")) {
			out.push(fragmentLine === "" ? "" : indent + fragmentLine);
		}
	}
	return out.join("\n");
};
