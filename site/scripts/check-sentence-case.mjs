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
 * A phrase starts at a capitalized word and runs until sentence or list
 * punctuation, or an opening bracket or quote. A word is reported when it
 * is not the first word of its phrase, looks like a regular capitalized
 * word ("Workspace"), and is not part of a proper noun from PROPER_NOUNS.
 * Lowercase words do not end a phrase, so both "Create a Template" and
 * "Browse other Templates" are reported. Acronyms ("SSH") and mixed-case
 * names ("GitHub") are never reported.
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
	"Community",
	"Dynamic Parameters",
	"Open in Coder",
	"Organization Workspace Access",
	"Premium",
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
	"Markdown",
	"Mermaid",
	"React",
	"Terraform",
	"Unicode",
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
	"Terminal Symbols",
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

// One segment of a hyphenated word: a regular capitalized word or a
// lowercase word. Words with a segment like "X" in "X-Forwarded-For" are
// left alone, since they are usually names.
const HYPHEN_SEGMENT = /^(?:[A-Z][a-z]+|[a-z]+)(?:['’][a-z]+)*$/;

/**
 * Returns the capitalized segments of a hyphenated word, such as "Based"
 * in "Role-Based", with their offsets in the word. Returns nothing when
 * the word has no hyphen or a segment is not a regular word.
 */
function capitalizedSegments(word) {
	const segments = word.split("-");
	if (segments.length < 2 || !segments.every((s) => HYPHEN_SEGMENT.test(s))) {
		return [];
	}
	const results = [];
	let offset = 0;
	for (const segment of segments) {
		if (/^[A-Z]/.test(segment)) {
			results.push({ offset, word: segment });
		}
		offset += segment.length + 1;
	}
	return results;
}

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
				// Also match a hyphenated compound like "Anthropic-compatible"
				// and a possessive like "Coder's".
				const word = tokens[i + j].word;
				return (
					word === w ||
					word.startsWith(`${w}-`) ||
					word === `${w}'s` ||
					word === `${w}’s`
				);
			});
			if (matches) {
				for (let j = 0; j < nounWords.length; j++) {
					properNounTokens.add(i + j);
				}
			}
		}
	}

	const results = [];
	let inPhrase = false;
	for (let i = 0; i < tokens.length; i++) {
		const token = tokens[i];
		const capitalized = /^[A-Z]/.test(token.word);
		if (token.raw.includes(SUBSTITUTION)) {
			// Unknown text; keep the current phrase, if any, going.
		} else if (!inPhrase || token.startsPhrase) {
			// A lowercase word outside a phrase may continue text from
			// elsewhere, so only a capitalized word starts a phrase.
			inPhrase = capitalized;
			if (capitalized && !properNounTokens.has(i)) {
				// Only the first segment of the first word keeps its
				// capital: "Browser-Only" becomes "Browser-only".
				for (const segment of capitalizedSegments(token.word).slice(1)) {
					results.push({
						offset: token.offset + segment.offset,
						word: segment.word,
					});
				}
			}
		} else if (properNounTokens.has(i)) {
			// Proper nouns keep their capitalization.
		} else if (capitalized && CAPITALIZED_WORD.test(token.word)) {
			results.push({ offset: token.offset, word: token.word });
		} else {
			for (const segment of capitalizedSegments(token.word)) {
				results.push({
					offset: token.offset + segment.offset,
					word: segment.word,
				});
			}
		}
		if (PHRASE_END.test(token.raw)) {
			inPhrase = false;
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

// Named HTML entities seen in UI text. JSX renders entities in text and
// attribute strings, so they are decoded before words are classified:
// "Validate&hellip;" reads as "Validate…". Other named entities are left
// as written.
const NAMED_ENTITIES = {
	amp: "&",
	apos: "'",
	copy: "©",
	gt: ">",
	hellip: "…",
	ldquo: "“",
	lsquo: "‘",
	lt: "<",
	mdash: "\u2014",
	minus: "−",
	nbsp: "\u00A0",
	ndash: "\u2013",
	quot: '"',
	rarr: "→",
	rdquo: "”",
	rsquo: "’",
};

const ENTITY = /&(?:#(\d+)|#[xX]([\da-fA-F]+)|([A-Za-z][\dA-Za-z]*));/y;

/** Returns the character an HTML entity renders, or null if unknown. */
function decodeEntity(match) {
	const [, decimal, hex, name] = match;
	const codePoint = decimal
		? Number.parseInt(decimal, 10)
		: hex
			? Number.parseInt(hex, 16)
			: null;
	if (codePoint === null) {
		return NAMED_ENTITIES[name] ?? null;
	}
	if (codePoint > 0x10ffff) {
		return null;
	}
	const char = String.fromCodePoint(codePoint);
	// Never let source text pose as a substitution.
	return char === SUBSTITUTION ? "\uFFFD" : char;
}

/**
 * Appends the source text in [start, end) to `target`, recording the
 * source position of each character. With `decodeEntities`, HTML
 * entities are replaced by the characters they render, each mapped to
 * the position of its "&".
 */
function appendSource(target, sourceText, start, end, decodeEntities) {
	let position = start;
	while (position < end) {
		if (decodeEntities && sourceText[position] === "&") {
			ENTITY.lastIndex = position;
			const match = ENTITY.exec(sourceText);
			const decoded = match && decodeEntity(match);
			if (decoded !== null && position + match[0].length <= end) {
				target.text += decoded;
				for (let i = 0; i < decoded.length; i++) {
					target.positions.push(position);
				}
				position += match[0].length;
				continue;
			}
		}
		target.text += sourceText[position];
		target.positions.push(position);
		position++;
	}
}

/**
 * Returns true when JSX renders the whitespace at one edge of a text
 * piece. JSX drops leading and trailing whitespace that contains a line
 * break.
 */
function rendersSpace(whitespace) {
	return whitespace.length > 0 && !whitespace.includes("\n");
}

// Caps the rendered variants checked for one JSX element. Past the cap,
// further conditional children are treated as substitutions and their
// JSX is checked on its own.
const MAX_JSX_VARIANTS = 64;

/**
 * Returns what an inline JSX expression may render: a JSX node, string
 * literal, or template literal to read as text, null for nothing, or "substitution" for an
 * unknown value. `cond && <b>Text</b>` renders the element or nothing,
 * `value || "Text"` and `value ?? "Text"` render the value or the
 * fallback, and a conditional renders either branch.
 */
function renderedOptions(expression) {
	let node = expression;
	while (ts.isParenthesizedExpression(node)) {
		node = node.expression;
	}
	if (
		ts.isJsxElement(node) ||
		ts.isJsxFragment(node) ||
		ts.isJsxSelfClosingElement(node) ||
		ts.isStringLiteral(node) ||
		ts.isNoSubstitutionTemplateLiteral(node) ||
		ts.isTemplateExpression(node)
	) {
		return [node];
	}
	if (ts.isBinaryExpression(node)) {
		const operator = node.operatorToken.kind;
		if (operator === ts.SyntaxKind.AmpersandAmpersandToken) {
			return [...renderedOptions(node.right), null];
		}
		// `label || "Fallback"` renders the left value or the fallback.
		if (
			operator === ts.SyntaxKind.BarBarToken ||
			operator === ts.SyntaxKind.QuestionQuestionToken
		) {
			return [...renderedOptions(node.left), ...renderedOptions(node.right)];
		}
	}
	if (ts.isConditionalExpression(node)) {
		return [
			...renderedOptions(node.whenTrue),
			...renderedOptions(node.whenFalse),
		];
	}
	if (
		node.kind === ts.SyntaxKind.NullKeyword ||
		node.kind === ts.SyntaxKind.FalseKeyword ||
		(ts.isIdentifier(node) && node.text === "undefined")
	) {
		return [null];
	}
	return ["substitution"];
}

/**
 * Returns the parts of a template expression in order: the source range
 * of each literal part, without its `, ${, and } delimiters, and a
 * substitution between each pair of parts.
 */
function templateParts(node, sourceFile) {
	const head = node.head;
	const parts = [{ start: head.getStart(sourceFile) + 1, end: head.end - 2 }];
	for (const span of node.templateSpans) {
		parts.push({ substitution: true });
		const literal = span.literal;
		const isTail = literal.kind === ts.SyntaxKind.TemplateTail;
		parts.push({
			start: literal.getStart(sourceFile) + 1,
			end: literal.end - (isTail ? 1 : 2),
		});
	}
	return parts;
}

/**
 * Collects the rendered text of a JSX element or fragment, including the
 * text of nested elements, so a phrase that crosses inline markup is
 * checked as one phrase: `Create <strong>New Workspace</strong>` reads as
 * "Create New Workspace". JSX that a child expression renders
 * conditionally is read in place too, once per rendered variant, so
 * `Create {show && <b>Workspace</b>}` reads as "Create Workspace" and as
 * "Create".
 *
 * The edge of a nested element joins the text on either side only when
 * JSX renders a space there; otherwise it ends the phrase, because
 * adjacent elements are usually separate labels. Nested nodes that are
 * read here are added to `consumed` so they are not checked again on
 * their own.
 */
function collectJsxTexts(container, sourceFile, consumed) {
	const sourceText = sourceFile.text;
	let variants = 1;

	// Appends the rendered pieces of `node` to `pieces`: text ranges,
	// substitutions for unknown values, boundaries at the edges of nested
	// elements, breaks for self-closing elements, and choices between the
	// pieces of each variant a conditional expression renders.
	const addNode = (node, pieces) => {
		if (node === null) {
			return;
		}
		if (node === "substitution") {
			pieces.push({ kind: "substitution" });
		} else if (ts.isJsxText(node)) {
			consumed.add(node);
			const text = sourceText.slice(node.pos, node.end);
			// Whitespace with a line break between tags renders nothing.
			if (!/^\s*$/.test(text) || rendersSpace(text)) {
				pieces.push({
					kind: "text",
					start: node.pos,
					end: node.end,
					decodeEntities: true,
				});
			}
		} else if (
			ts.isStringLiteral(node) ||
			ts.isNoSubstitutionTemplateLiteral(node)
		) {
			consumed.add(node);
			pieces.push({
				kind: "text",
				start: node.getStart(sourceFile) + 1,
				end: node.end - 1,
			});
		} else if (ts.isTemplateExpression(node)) {
			consumed.add(node);
			for (const { start, end, substitution } of templateParts(
				node,
				sourceFile,
			)) {
				pieces.push(
					substitution
						? { kind: "substitution", inline: true }
						: { kind: "text", start, end },
				);
			}
		} else if (ts.isJsxExpression(node)) {
			// An empty expression, such as {/* note */}, renders nothing.
			if (node.expression) {
				addExpression(node.expression, pieces);
			}
		} else if (ts.isJsxElement(node) || ts.isJsxFragment(node)) {
			consumed.add(node);
			pieces.push({ kind: "boundary", edge: "open" });
			for (const child of node.children) {
				addNode(child, pieces);
			}
			pieces.push({ kind: "boundary", edge: "close" });
		} else {
			// A self-closing element, such as an icon, sits between
			// separate labels more often than inside a phrase.
			pieces.push({ kind: "break" });
		}
	};

	const addExpression = (expression, pieces) => {
		const options = renderedOptions(expression);
		if (options.length === 1) {
			addNode(options[0], pieces);
		} else if (
			options.every((option) => option === "substitution") ||
			variants * options.length > MAX_JSX_VARIANTS
		) {
			pieces.push({ kind: "substitution" });
		} else {
			variants *= options.length;
			pieces.push({
				kind: "choice",
				options: options.map((option) => {
					const optionPieces = [];
					addNode(option, optionPieces);
					return optionPieces;
				}),
			});
		}
	};

	const rootPieces = [];
	for (const child of container.children) {
		addNode(child, rootPieces);
	}

	const texts = [];
	for (const pieces of expandChoices(rootPieces)) {
		texts.push(...joinJsxPieces(pieces, sourceText));
	}
	return texts;
}

/** Expands choice pieces into every linear sequence of pieces. */
function expandChoices(pieces) {
	let sequences = [[]];
	for (const piece of pieces) {
		if (piece.kind !== "choice") {
			for (const sequence of sequences) {
				sequence.push(piece);
			}
			continue;
		}
		const next = [];
		for (const option of piece.options) {
			for (const tail of expandChoices(option)) {
				for (const sequence of sequences) {
					next.push([...sequence, ...tail]);
				}
			}
		}
		sequences = next;
	}
	return sequences;
}

/**
 * Joins a linear sequence of JSX pieces into phrases, each with its text
 * and the source position of each character.
 */
function joinJsxPieces(pieces, sourceText) {
	const edgeRendersSpace = (piece, edge) => {
		if (piece?.kind !== "text") {
			return false;
		}
		const text = sourceText.slice(piece.start, piece.end);
		const whitespace = text.match(edge === "end" ? /\s*$/ : /^\s*/)[0];
		return rendersSpace(whitespace);
	};

	const texts = [];
	let current = { text: "", positions: [] };
	const endPhrase = () => {
		if (current.text.trim()) {
			texts.push(current);
		}
		current = { text: "", positions: [] };
	};
	for (let i = 0; i < pieces.length; i++) {
		const piece = pieces[i];
		if (
			piece.kind === "boundary" &&
			piece.edge === "open" &&
			pieces[i - 1]?.edge === "close"
		) {
			// Sibling elements with nothing rendered between them are
			// separate labels, whatever whitespace they hold inside.
			endPhrase();
		} else if (piece.kind === "boundary") {
			let before = i - 1;
			while (pieces[before]?.kind === "boundary") {
				before--;
			}
			let after = i + 1;
			while (pieces[after]?.kind === "boundary") {
				after++;
			}
			if (
				!edgeRendersSpace(pieces[before], "end") &&
				!edgeRendersSpace(pieces[after], "start")
			) {
				endPhrase();
			}
		} else if (piece.kind === "break") {
			endPhrase();
		} else if (piece.kind === "substitution" && piece.inline) {
			// A template substitution joins the literal text around it,
			// as in `${count}s`.
			current.text += SUBSTITUTION;
			current.positions.push(-1);
		} else if (piece.kind === "substitution") {
			// Pad with spaces so the substitution is a token of its own.
			current.text += ` ${SUBSTITUTION} `;
			current.positions.push(-1, -1, -1);
		} else {
			appendSource(
				current,
				sourceText,
				piece.start,
				piece.end,
				piece.decodeEntities,
			);
		}
	}
	endPhrase();
	return texts;
}

/**
 * Collects every displayed text node as its text plus the source position
 * of each character. Substitutions in template literals and JSX map to -1.
 */
function collectTexts(sourceFile) {
	const sourceText = sourceFile.text;
	const texts = [];
	// JSX nodes already read as part of an enclosing element's text.
	const consumed = new Set();
	const addRange = (start, end, decodeEntities) => {
		const text = { text: "", positions: [] };
		appendSource(text, sourceText, start, end, decodeEntities);
		texts.push(text);
	};
	const visit = (node) => {
		switch (node.kind) {
			case ts.SyntaxKind.JsxElement:
			case ts.SyntaxKind.JsxFragment:
				if (!consumed.has(node)) {
					texts.push(...collectJsxTexts(node, sourceFile, consumed));
				}
				break;
			case ts.SyntaxKind.StringLiteral:
			case ts.SyntaxKind.NoSubstitutionTemplateLiteral:
				if (!consumed.has(node) && !isNonDisplayLiteral(node)) {
					// JSX renders entities in attribute strings only.
					addRange(
						node.getStart(sourceFile),
						node.end,
						ts.isJsxAttribute(node.parent),
					);
				}
				break;
			case ts.SyntaxKind.TemplateExpression: {
				if (consumed.has(node) || isNonDisplayLiteral(node)) {
					break;
				}
				const text = { text: "", positions: [] };
				for (const { start, end, substitution } of templateParts(
					node,
					sourceFile,
				)) {
					if (substitution) {
						text.text += SUBSTITUTION;
						text.positions.push(-1);
					} else {
						appendSource(text, sourceText, start, end, false);
					}
				}
				texts.push(text);
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
	// Conditional JSX is checked once per rendered variant, so the same
	// word can be reported more than once.
	const reportedPositions = new Set();
	for (const { text, positions } of collectTexts(sourceFile)) {
		for (const { offset, word } of findTitleCaseWords(text)) {
			const position = positions[offset];
			if (reportedPositions.has(position)) {
				continue;
			}
			reportedPositions.add(position);
			const { line, character } =
				sourceFile.getLineAndCharacterOfPosition(position);
			if (expectedLines.has(line)) {
				usedExpectations.add(line);
				continue;
			}
			issues.push({ position, line: line + 1, column: character + 1, word });
		}
	}
	issues.sort((a, b) => a.position - b.position);

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
