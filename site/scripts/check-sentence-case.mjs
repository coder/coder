/**
 * Sentence case checker.
 *
 * The UI uses sentence case: capitalize the first word of a phrase and
 * proper nouns only ("Create workspace", not "Create Workspace"). This
 * script reports user-facing text in src/ that uses title case instead.
 *
 * Text is read from the TypeScript AST: JSX text plus string and template
 * literals. Literals that are not displayed text are skipped: module
 * specifiers, type positions, property names, tagged templates, and
 * operands of equality checks or `case` clauses (those usually compare
 * against API values).
 *
 * A phrase is a run of consecutive words that start with an uppercase
 * letter, including lowercase connectors such as "a" or "of" between them
 * ("Create a Template"). A word is reported when it is not the first word
 * of its run, looks like a regular capitalized word ("Workspace"), and is
 * not part of a proper noun from PROPER_NOUNS. Acronyms ("SSH") and
 * mixed-case names ("GitHub") are never reported.
 *
 * For deliberate exceptions, put a comment containing
 * "sentence-case-expect" on the line above the text. Like
 * @ts-expect-error, the check fails when that line has nothing to report,
 * so stale exceptions do not accumulate.
 *
 * Usage:  node scripts/check-sentence-case.mjs [--fix]
 */
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const EXPECT_COMMENT = "sentence-case-expect";

// Names that keep their capitalization. Single words only need listing
// when they look like regular words; acronyms and mixed-case names such
// as "SSH" or "GitHub" are already allowed.
const PROPER_NOUNS = [
	// Coder products, features, and built-in role names.
	"Agent Hours",
	"Agent hours",
	"AI Gateway",
	"AI Gateway Proxy",
	"AI Governance",
	"Coder",
	"Coder Agents",
	"Coder Agents User",
	"Codernauts",
	"Coder Desktop",
	"Coder Privacy Policy",
	"Coder Registry",
	"Coder Technologies",
	"Coder Workspaces",
	"Dynamic Parameters",
	"Organization Workspace Access",
	"Template Admin",
	"Terms of Service",

	// Days of the week.
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
	"Sunday",

	// Third-party products and companies.
	"Splunk",
	"Anthropic",
	"Azure Repos",
	"Bedrock",
	"Claude Platform",
	"Copilot",
	"Discord",
	"Git",
	"Google Cloud",
	"Inc",
	"JetBrains Gateway",
	"JetBrains Toolbox",
	"Terraform",
	"Unix",
	"VS Code",
	"VS Code Desktop",
	"VS Code Insiders",
	"VS Code Remote SSH",
	"VSCode Insiders",
	"Visual Studio Code",

	// Standards and protocols.
	"Cross-Origin Resource Sharing",
	"Dynamic Client Registration",
	"NAT Port Mapping Protocol",
	"OpenID Connect",
	"Port Control Protocol",
	"Universal Plug and Play",

	// Font names, shown in the terminal font picker.
	"Coder Terminal Symbols",
	"Courier New",
	"Fira Code",
	"Geist Mono",
	"Geist Mono Variable",
	"Geist Variable",
	"IBM Plex Mono",
	"JetBrains Mono",
	"Liberation Mono",
	"Lucida Console",
	"Lucida Sans Typewriter",
	"Source Code Pro",
];

// Generated code, tests, and fixture data, plus files whose text is
// mostly names (people and job titles).
const EXCLUDED_FILE_PATTERNS = [
	/Generated\.ts$/,
	/\.(stories|test|jest)\.tsx?$/,
	/(^|\/)(mocks|storybookData|storybookUtils)\.tsx?$/,
	/(^|\/)(testHelpers|storybookData)\//,
	/[fF]ixtures(\.tsx?$|\/)/,
	/^src\/pages\/CoderCupPage\/roster\.ts$/,
];

const properNounWords = PROPER_NOUNS.map((noun) => noun.split(" "));

// Surrounding punctuation that is not part of a word, such as quotes,
// brackets, and sentence punctuation.
const LEADING_PUNCTUATION = /^[^\p{L}\p{N}]+/u;
const TRAILING_PUNCTUATION = /[^\p{L}\p{N}]+$/u;

// A regular capitalized word, like "Workspace" or "Don't".
const CAPITALIZED_WORD = /^[A-Z][a-z]+(?:['’-][a-z]+)*$/;

// Lowercase words that title case leaves uncapitalized. They continue a
// phrase, so "Create a Template" is reported like "Create Template".
const CONNECTORS = new Set([
	"a",
	"an",
	"and",
	"as",
	"at",
	"by",
	"for",
	"from",
	"in",
	"of",
	"on",
	"or",
	"the",
	"to",
	"with",
]);

// Punctuation that ends a sentence, label, or list item, so the next
// word starts a new phrase and may be capitalized ("GitHub, GitLab").
const PHRASE_END = /[.!?:;,]["'’)\]]*$/;

/**
 * Returns the title case words in `text`. Each result has the word's
 * offset in `text` and the word itself.
 */
export function findTitleCaseWords(text) {
	const tokens = [];
	for (const match of text.matchAll(/\S+/g)) {
		const leading = match[0].match(LEADING_PUNCTUATION)?.[0] ?? "";
		const word = match[0]
			.slice(leading.length)
			.replace(TRAILING_PUNCTUATION, "");
		tokens.push({
			offset: match.index + leading.length,
			raw: match[0],
			word,
			// A leading bracket or quote starts a new phrase, e.g.
			// "Use (Recommended)" is a label followed by a note.
			startsPhrase: leading.length > 0,
		});
	}

	// Indexes of tokens that belong to a proper noun.
	const properNounTokens = new Set();
	for (const nounWords of properNounWords) {
		for (let i = 0; i + nounWords.length <= tokens.length; i++) {
			const matches = nounWords.every((w, j) => {
				// Also match a hyphenated compound like "Anthropic-compatible".
				const word = tokens[i + j].word;
				return word === w || word.startsWith(`${w}-`);
			});
			if (matches) {
				for (let j = 0; j < nounWords.length; j++) {
					properNounTokens.add(i + j);
				}
			}
		}
	}

	const results = [];
	let phraseStart = -1;
	for (let i = 0; i < tokens.length; i++) {
		const token = tokens[i];
		const capitalized = /^[A-Z]/.test(token.word);
		if (token.raw.includes(SUBSTITUTION)) {
			// Unknown text; keep the current phrase, if any, going.
		} else if (
			phraseStart !== -1 &&
			!token.startsPhrase &&
			CONNECTORS.has(token.word)
		) {
			// Keep the current phrase going.
		} else if (!capitalized || token.startsPhrase) {
			phraseStart = capitalized ? i : -1;
		} else if (phraseStart === -1) {
			phraseStart = i;
		} else if (CAPITALIZED_WORD.test(token.word) && !properNounTokens.has(i)) {
			results.push({ offset: token.offset, word: token.word });
		}
		if (PHRASE_END.test(token.raw)) {
			phraseStart = -1;
		}
	}
	return results;
}

/** Returns true when the literal is code rather than displayed text. */
function isNonDisplayLiteral(node) {
	const parent = node.parent;
	if (
		ts.isImportDeclaration(parent) ||
		ts.isExportDeclaration(parent) ||
		ts.isExternalModuleReference(parent) ||
		ts.isLiteralTypeNode(parent) ||
		ts.isCaseClause(parent) ||
		ts.isElementAccessExpression(parent) ||
		ts.isTaggedTemplateExpression(parent)
	) {
		return true;
	}
	if (
		ts.isCallExpression(parent) &&
		parent.expression.kind === ts.SyntaxKind.ImportKeyword
	) {
		return true;
	}
	if ("name" in parent && parent.name === node) {
		return true;
	}
	if (ts.isBinaryExpression(parent)) {
		const kind = parent.operatorToken.kind;
		return (
			kind === ts.SyntaxKind.EqualsEqualsEqualsToken ||
			kind === ts.SyntaxKind.ExclamationEqualsEqualsToken ||
			kind === ts.SyntaxKind.EqualsEqualsToken ||
			kind === ts.SyntaxKind.ExclamationEqualsToken
		);
	}
	return false;
}

// Stands in for a template substitution. A substitution continues a
// phrase but does not start one, so `New ${name} Provider` is reported
// while `${message} To continue` is not, because the substitution may
// end a sentence.
const SUBSTITUTION = "\uE000";

/**
 * Collects every displayed text node as its text plus the source position
 * of each character. Substitutions in template literals map to -1.
 */
function collectTexts(sourceFile) {
	const sourceText = sourceFile.text;
	const texts = [];
	const addRange = (start, end) => {
		texts.push({
			text: sourceText.slice(start, end),
			positions: Array.from({ length: end - start }, (_, i) => start + i),
		});
	};
	const visit = (node) => {
		switch (node.kind) {
			case ts.SyntaxKind.JsxText:
				addRange(node.getStart(sourceFile), node.end);
				break;
			case ts.SyntaxKind.StringLiteral:
			case ts.SyntaxKind.NoSubstitutionTemplateLiteral:
				if (!isNonDisplayLiteral(node)) {
					addRange(node.getStart(sourceFile), node.end);
				}
				break;
			case ts.SyntaxKind.TemplateExpression: {
				if (isNonDisplayLiteral(node)) {
					break;
				}
				// Join the literal parts without their `, ${, and } delimiters.
				let text = "";
				const positions = [];
				const addPart = (start, end) => {
					text += sourceText.slice(start, end);
					for (let i = start; i < end; i++) {
						positions.push(i);
					}
				};
				const head = node.head;
				addPart(head.getStart(sourceFile) + 1, head.end - 2);
				for (const span of node.templateSpans) {
					text += SUBSTITUTION;
					positions.push(-1);
					const literal = span.literal;
					const isTail = literal.kind === ts.SyntaxKind.TemplateTail;
					addPart(
						literal.getStart(sourceFile) + 1,
						literal.end - (isTail ? 1 : 2),
					);
				}
				texts.push({ text, positions });
				break;
			}
		}
		ts.forEachChild(node, visit);
	};
	visit(sourceFile);
	return texts;
}

/**
 * Checks one file's source text. Returns the reported words, the
 * line numbers of unused expect comments, and the source with every
 * reported word lowercased.
 */
export function checkSource(fileName, sourceText) {
	const sourceFile = ts.createSourceFile(
		fileName,
		sourceText,
		ts.ScriptTarget.Latest,
		true,
		fileName.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
	);

	// Expect comments apply to the line after the comment. Line numbers
	// from the TypeScript API are zero-based; results are one-based.
	const expectedLines = new Set();
	sourceText.split("\n").forEach((line, index) => {
		if (line.includes(EXPECT_COMMENT)) {
			expectedLines.add(index + 1);
		}
	});

	const issues = [];
	const usedExpectations = new Set();
	for (const { text, positions } of collectTexts(sourceFile)) {
		for (const { offset, word } of findTitleCaseWords(text)) {
			const position = positions[offset];
			const { line, character } =
				sourceFile.getLineAndCharacterOfPosition(position);
			if (expectedLines.has(line)) {
				usedExpectations.add(line);
				continue;
			}
			issues.push({ position, line: line + 1, column: character + 1, word });
		}
	}

	let fixedText = sourceText;
	for (const { position } of [...issues].reverse()) {
		fixedText =
			fixedText.slice(0, position) +
			fixedText[position].toLowerCase() +
			fixedText.slice(position + 1);
	}

	// A zero-based expected line is also the one-based line of its
	// comment, which is the line to report.
	const unusedExpectations = [...expectedLines].filter(
		(line) => !usedExpectations.has(line),
	);

	return { issues, unusedExpectations, fixedText };
}

function collectFiles(dir) {
	const files = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const path = join(dir, entry.name);
		if (entry.isDirectory()) {
			files.push(...collectFiles(path));
		} else if (/\.tsx?$/.test(entry.name)) {
			files.push(path);
		}
	}
	return files;
}

function isExcluded(path) {
	return EXCLUDED_FILE_PATTERNS.some((pattern) => pattern.test(path));
}

// Only run the main block when executed directly, not when imported by
// tests for the exported functions.
if (process.argv[1] === fileURLToPath(import.meta.url)) {
	const siteDir = fileURLToPath(new URL("..", import.meta.url));
	const fix = process.argv.includes("--fix");
	const files = collectFiles(join(siteDir, "src"))
		.map((path) => relative(siteDir, path))
		.filter((path) => !isExcluded(path));

	let issueCount = 0;
	let unusedCount = 0;
	for (const file of files) {
		const sourceText = readFileSync(join(siteDir, file), "utf8");
		const { issues, unusedExpectations, fixedText } = checkSource(
			file,
			sourceText,
		);
		for (const { line, column, word } of issues) {
			console.log(
				`${file}:${line}:${column}: "${word}" should be lowercase in sentence case`,
			);
		}
		for (const line of unusedExpectations) {
			console.log(
				`${file}:${line}: unused "${EXPECT_COMMENT}" comment, the next line has no title case text`,
			);
		}
		issueCount += issues.length;
		unusedCount += unusedExpectations.length;
		if (fix && issues.length > 0) {
			writeFileSync(join(siteDir, file), fixedText);
		}
	}

	if (fix && issueCount > 0) {
		console.log(`\nLowercased ${issueCount} word(s). Review the changes.`);
	}
	if ((!fix && issueCount > 0) || unusedCount > 0) {
		console.log(
			[
				"",
				"Use sentence case for UI text: capitalize only the first word and proper nouns.",
				"To fix automatically: pnpm run lint:sentence-case --fix",
				"For proper nouns: add them to PROPER_NOUNS in scripts/check-sentence-case.mjs.",
				`For other exceptions: add a "${EXPECT_COMMENT}" comment on the line above.`,
			].join("\n"),
		);
		process.exitCode = 1;
	}
}
