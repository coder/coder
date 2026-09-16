// Every profile-derived string (function names, file paths, label keys and
// values, target status text) passes through this module before it is stored,
// rendered, or sent to the model.

export const MAX_STRING_LENGTH = 160;
export const MAX_FUNCTION_NAME_LENGTH = 512;
const TRUNCATION_MARKER = "...";
export const MAX_REGEX_LENGTH = 128;
export const MAX_REGEX_QUANTIFIERS = 3;

// Control characters (C0, DEL, C1) and Unicode line and paragraph separators.
const CONTROL_CHARS = /[\u0000-\u001f\u007f-\u009f\u2028\u2029]/g;

/**
 * Removes control characters and newlines and caps the result at maxLength
 * characters, ending with a truncation marker when cut. Non-string input
 * yields an empty string.
 */
export function normalizeString(
	input: unknown,
	maxLength = MAX_STRING_LENGTH,
): string {
	if (typeof input !== "string") {
		return "";
	}
	const cleaned = input.replace(CONTROL_CHARS, "");
	if (cleaned.length <= maxLength) {
		return cleaned;
	}
	return (
		cleaned.slice(0, Math.max(0, maxLength - TRUNCATION_MARKER.length)) +
		TRUNCATION_MARKER
	);
}

/**
 * Reduces an absolute source path to its last two segments (directory and
 * file name), which is how pprof output is usually read. Paths with fewer
 * segments are returned unchanged. The result is normalized.
 */
export function trimPath(path: string): string {
	const normalized = normalizeString(path);
	const parts = normalized.split("/").filter((p) => p.length > 0);
	if (parts.length === 0) {
		return normalized;
	}
	return parts.slice(-2).join("/");
}

const REGEX_HINT =
	"Use a plain substring such as main. or a simple pattern such as ^lab\\..*Blob$.";

/**
 * Compiles a user-supplied focus pattern into a RegExp under a bounded
 * backtracking policy: the pattern is at most MAX_REGEX_LENGTH characters,
 * contains at most MAX_REGEX_QUANTIFIERS quantifiers, applies no quantifier
 * to a group, and uses no lookaround or backreferences. Patterns that pass
 * run against strings capped at MAX_FUNCTION_NAME_LENGTH characters. This
 * excludes the common catastrophic shapes; it is not a proof of linear
 * matching. Rejected or invalid patterns throw an Error whose message is
 * safe to show to the caller.
 */
export function safeRegex(pattern: string): RegExp {
	if (pattern.length > MAX_REGEX_LENGTH) {
		throw new Error(
			`Invalid focus pattern: longer than ${MAX_REGEX_LENGTH} characters. ${REGEX_HINT}`,
		);
	}
	const problem = checkRegexPolicy(pattern);
	if (problem !== undefined) {
		throw new Error(`Invalid focus pattern: ${problem}. ${REGEX_HINT}`);
	}
	try {
		return new RegExp(pattern);
	} catch (err) {
		const reason = err instanceof Error ? err.message : String(err);
		throw new Error(`Invalid focus pattern: ${reason}. ${REGEX_HINT}`);
	}
}

const QUANTIFIER_START = new Set(["*", "+", "?", "{"]);

// Scans the pattern outside character classes and escapes. Returns a
// description of the first policy violation, or undefined when the pattern
// is acceptable.
function checkRegexPolicy(pattern: string): string | undefined {
	let quantifiers = 0;
	let inClass = false;
	for (let i = 0; i < pattern.length; i++) {
		const ch = pattern[i];
		if (ch === "\\") {
			const next = pattern[i + 1];
			if (next !== undefined && next >= "1" && next <= "9") {
				return "backreferences such as \\1 are not allowed";
			}
			i++;
			continue;
		}
		if (inClass) {
			if (ch === "]") {
				inClass = false;
			}
			continue;
		}
		switch (ch) {
			case "[":
				inClass = true;
				break;
			case "(":
				if (
					pattern.startsWith("(?=", i) ||
					pattern.startsWith("(?!", i) ||
					pattern.startsWith("(?<=", i) ||
					pattern.startsWith("(?<!", i)
				) {
					return "lookaround assertions are not allowed";
				}
				break;
			case ")": {
				const next = pattern[i + 1];
				if (next !== undefined && QUANTIFIER_START.has(next)) {
					return "quantifiers applied to a group such as (ab)+ are not allowed";
				}
				break;
			}
			default:
				if (QUANTIFIER_START.has(ch)) {
					// A quantifier's own lazy or brace suffix is part of it.
					if (ch === "?" && i > 0 && QUANTIFIER_START.has(pattern[i - 1])) {
						break;
					}
					if (ch === "{" && !/^\{\d+(,\d*)?\}/.test(pattern.slice(i))) {
						break;
					}
					quantifiers++;
					if (quantifiers > MAX_REGEX_QUANTIFIERS) {
						return `more than ${MAX_REGEX_QUANTIFIERS} quantifiers`;
					}
				}
		}
	}
	return undefined;
}
