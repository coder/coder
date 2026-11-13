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
	"Web Terminal",
	"Workspace Proxy",

	// Keyboard keys, named in keyboard instructions.
	"Enter",
	"Escape",
	"Space",

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
	"Amazon Bedrock",
	"Bedrock",
	"Claude",
	"Claude Platform",
	"Bitbucket",
	"Copilot",
	"Datadog",
	"Discord",
	"Git",
	"Google Cloud",
	"Inc",
	"JetBrains Gateway",
	"JetBrains Toolbox",
	"Mantle",
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
	"Consolas",
	"Courier",
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
	"Menlo",
	"Monaco",
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
const LEADING_PUNCTUATION = /^[^\p{L}\p{N}\p{M}]+/u;
const TRAILING_PUNCTUATION = /[^\p{L}\p{N}\p{M}]+$/u;

// A regular capitalized word, like "Workspace" or "Don't".
// Letters may carry combining marks, as in decomposed "Re\u0301sume\u0301".
const CAPITALIZED_WORD =
	/^\p{Lu}\p{M}*(?:\p{Ll}\p{M}*)+(?:['’-](?:\p{Ll}\p{M}*)+)*$/u;

// One segment of a hyphenated word: a regular capitalized word or a
// lowercase word. Words with a segment like "X" in "X-Forwarded-For" are
// left alone, since they are usually names.
const HYPHEN_SEGMENT =
	/^(?:\p{Lu}\p{M}*(?:\p{Ll}\p{M}*)+|(?:\p{Ll}\p{M}*)+)(?:['’](?:\p{Ll}\p{M}*)+)*$/u;

/**
 * Returns the capitalized segments of a hyphenated word, such as "Based"
 * in "Role-Based", with their offsets in the word. Segments before
 * `start` are skipped, such as a proper noun prefix. Returns nothing
 * when the word has no hyphen or a checked segment is not a regular word.
 */
function capitalizedSegments(word, start = 0) {
	if (!word.includes("-")) {
		return [];
	}
	const segments = word.slice(start).split("-");
	if (!segments.every((s) => HYPHEN_SEGMENT.test(s))) {
		return [];
	}
	const results = [];
	let offset = start;
	for (const segment of segments) {
		if (/^\p{Lu}/u.test(segment)) {
			results.push({ offset, word: segment });
		}
		offset += segment.length + 1;
	}
	return results;
}

// Words that follow a single-letter name, such as a key in "press A to
// rotate" or an option in "plan A or B", rather than the article "a".
const AFTER_LETTER_NAME = new Set([
	"and",
	"as",
	"at",
	"by",
	"for",
	"from",
	"in",
	"is",
	"key",
	"of",
	"on",
	"or",
	"to",
	"with",
]);

/**
 * Returns true when `token` is the article "A" mid-phrase: a bare "A"
 * followed directly by a lowercase word that does not suggest a name.
 */
function isArticleA(token, next) {
	return (
		token.word === "A" &&
		token.raw === "A" &&
		next !== undefined &&
		!next.startsPhrase &&
		/^\p{Ll}+$/u.test(next.word) &&
		!AFTER_LETTER_NAME.has(next.word)
	);
}

// Punctuation that joins the words around it into one phrase. Other
// standalone punctuation, such as the "-" in "Workspace - Health",
// separates labels.
const CONNECTOR = /^[&/+]$/;

// Brackets and quotes that open or close an aside. The phrase around an
// aside resumes after it closes: "Create a (temporary) workspace".
const OPENING_MARK = /[([{“‘"']/;
const CLOSING_MARK = /[)\]}”’"']/;

// Punctuation that ends a sentence or label, so the next word starts a
// new phrase and may be capitalized. Commas and semicolons continue the
// sentence ("Create a workspace; then open settings"); lists of names
// such as "Menlo, Monaco" rely on PROPER_NOUNS.
const PHRASE_END = /[.!?:…][)\]}”’"']*$/;

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
		const trailing = match[0].slice(leading.length + word.length);
		tokens.push({
			offset: match.index + leading.length,
			raw: match[0],
			word,
			// A leading bracket or quote starts a new phrase, e.g.
			// "Use (Recommended)" is a label followed by a note.
			startsPhrase: leading.length > 0,
			opens: OPENING_MARK.test(leading),
			closes: CLOSING_MARK.test(trailing),
		});
	}

	// Indexes of tokens that belong to a proper noun. A token like
	// "Coder-Based" also maps to the offset of the suffix after the proper
	// noun, which is still checked.
	const properNounTokens = new Set();
	const properNounSuffixes = new Map();
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
					const word = tokens[i + j].word;
					if (word.startsWith(`${nounWords[j]}-`)) {
						properNounSuffixes.set(i + j, nounWords[j].length + 1);
					}
				}
			}
		}
	}

	const results = [];
	const reportSegments = (token, start) => {
		for (const segment of capitalizedSegments(token.word, start)) {
			results.push({
				offset: token.offset + segment.offset,
				word: segment.word,
			});
		}
	};
	let inPhrase = false;
	// The phrase state around each open aside.
	const outerPhrases = [];
	for (let i = 0; i < tokens.length; i++) {
		const token = tokens[i];
		if (token.opens) {
			outerPhrases.push(inPhrase);
		}
		const capitalized = /^\p{Lu}/u.test(token.word);
		if (token.raw.includes(SUBSTITUTION)) {
			// Unknown text; keep the current phrase, if any, going.
		} else if (CONNECTOR.test(token.raw)) {
			// "Create & manage" and "Create / manage" are one phrase.
		} else if (!inPhrase || token.startsPhrase) {
			// A lowercase word outside a phrase may continue text from
			// elsewhere, so only a capitalized word starts a phrase.
			inPhrase = capitalized;
			if (properNounSuffixes.has(i)) {
				reportSegments(token, properNounSuffixes.get(i));
			} else if (capitalized && !properNounTokens.has(i)) {
				// Only the first segment of the first word keeps its
				// capital: "Browser-Only" becomes "Browser-only".
				for (const segment of capitalizedSegments(token.word).slice(1)) {
					results.push({
						offset: token.offset + segment.offset,
						word: segment.word,
					});
				}
			}
		} else if (properNounSuffixes.has(i)) {
			// The proper noun keeps its capital, but not the suffix:
			// "Coder-Based" becomes "Coder-based".
			reportSegments(token, properNounSuffixes.get(i));
		} else if (properNounTokens.has(i)) {
			// Proper nouns keep their capitalization.
		} else if (
			(capitalized && CAPITALIZED_WORD.test(token.word)) ||
			isArticleA(token, tokens[i + 1])
		) {
			results.push({ offset: token.offset, word: token.word });
		} else {
			reportSegments(token, 0);
		}
		if (token.closes && outerPhrases.length > 0) {
			inPhrase = outerPhrases.pop();
		}
		if (PHRASE_END.test(token.raw)) {
			inPhrase = false;
		}
	}
	return results;
}

// Attributes whose values are code rather than displayed text: IDs and
// ID references, URLs, enumerated values, CSS, and form values. All
// data-* attributes and the aria-* attributes not listed in
// ARIA_TEXT_ATTRIBUTES are code too.
const CODE_ATTRIBUTES = new Set([
	"accept",
	"acceptCharset",
	"accessKey",
	"action",
	"allow",
	"as",
	"autoCapitalize",
	"autoComplete",
	"charSet",
	"className",
	"crossOrigin",
	"dangerouslySetInnerHTML",
	"d",
	"decoding",
	"defaultValue",
	"dir",
	"download",
	"encType",
	"enterKeyHint",
	"fill",
	"form",
	"formAction",
	"formEncType",
	"formMethod",
	"formTarget",
	"headers",
	"href",
	"hrefLang",
	"htmlFor",
	"id",
	"inputMode",
	"integrity",
	"is",
	"itemID",
	"itemProp",
	"itemRef",
	"itemScope",
	"itemType",
	"key",
	"kind",
	"lang",
	"list",
	"loading",
	"media",
	"method",
	"name",
	"nonce",
	"part",
	"pattern",
	"ping",
	"points",
	"referrerPolicy",
	"rel",
	"role",
	"sandbox",
	"scope",
	"shape",
	"sizes",
	"slot",
	"src",
	"srcDoc",
	"srcLang",
	"srcSet",
	"stroke",
	"style",
	"target",
	"to",
	"transform",
	"type",
	"useMap",
	"value",
	"viewBox",
	"wrap",
	"xmlns",
]);

const ARIA_TEXT_ATTRIBUTES = new Set([
	"aria-description",
	"aria-label",
	"aria-placeholder",
	"aria-roledescription",
	"aria-valuetext",
]);

// Attributes whose values are code on components as well as on
// intrinsic elements, by React convention. Other names in
// CODE_ATTRIBUTES, such as `name`, are often displayed text on components.
const COMPONENT_CODE_ATTRIBUTES = new Set([
	"className",
	"dangerouslySetInnerHTML",
	"defaultValue",
	"href",
	"htmlFor",
	"id",
	"key",
	"rel",
	"role",
	"src",
	"srcSet",
	"style",
	"target",
	"to",
	"type",
	"value",
]);

/**
 * Returns the JSX attribute that `node` is part of, as its name and
 * whether its element is intrinsic, such as "div", or null. Looks through
 * any expression in the value, such as `className={cn("a", { b: ok })}`
 * or a callback, and through properties of spread objects, such as
 * `{...{ className: "a" }}`. Stops at JSX elements, whose own attributes
 * and children are checked on their own.
 */
function enclosingJsxAttribute(node) {
	let property = null;
	let parent = node.parent;
	while (
		parent &&
		!ts.isJsxAttribute(parent) &&
		!ts.isJsxSpreadAttribute(parent) &&
		!ts.isJsxElement(parent) &&
		!ts.isJsxSelfClosingElement(parent) &&
		!ts.isJsxFragment(parent) &&
		!ts.isSourceFile(parent)
	) {
		if (ts.isPropertyAssignment(parent)) {
			property = parent;
		}
		parent = parent.parent;
	}
	let name;
	if (parent && ts.isJsxAttribute(parent)) {
		name = parent.name.getText();
	} else if (
		parent &&
		ts.isJsxSpreadAttribute(parent) &&
		property &&
		isSpreadObject(property.parent, parent)
	) {
		// The outermost property of the spread object names the attribute.
		name = staticPropertyName(property.name);
	} else {
		return null;
	}
	const tagName = parent.parent.parent.tagName;
	const intrinsic = ts.isIdentifier(tagName) && /^[a-z]/.test(tagName.text);
	return { name, intrinsic };
}

/**
 * Returns the static text of a property name: `className`, "className",
 * and ["className"] all return className.
 */
function staticPropertyName(name) {
	const key = ts.isComputedPropertyName(name)
		? skipTransparent(name.expression)
		: name;
	return ts.isIdentifier(key) ||
		ts.isStringLiteral(key) ||
		ts.isNoSubstitutionTemplateLiteral(key) ||
		ts.isNumericLiteral(key)
		? key.text
		: name.getText();
}

/**
 * Returns true when `object` is what `spread` spreads: the object itself
 * or a branch of a conditional or logical expression, such as
 * `{...(ok ? { className: "a" } : {})}`.
 */
function isSpreadObject(object, spread) {
	let node = object;
	while (
		isTransparent(node.parent) ||
		ts.isConditionalExpression(node.parent) ||
		(ts.isBinaryExpression(node.parent) &&
			[
				ts.SyntaxKind.AmpersandAmpersandToken,
				ts.SyntaxKind.BarBarToken,
				ts.SyntaxKind.QuestionQuestionToken,
			].includes(node.parent.operatorToken.kind))
	) {
		node = node.parent;
	}
	return ts.isObjectLiteralExpression(object) && node.parent === spread;
}

/**
 * Returns true when a JSX attribute's value is code, like data-state.
 * DOM attributes such as `name` are code only on intrinsic elements.
 */
function isCodeAttribute({ name, intrinsic }) {
	return (
		(intrinsic ? CODE_ATTRIBUTES : COMPONENT_CODE_ATTRIBUTES).has(name) ||
		name.startsWith("data-") ||
		(name.startsWith("aria-") && !ARIA_TEXT_ATTRIBUTES.has(name))
	);
}

/**
 * Returns true when `node` is inside the value of an object property
 * whose key names code, such as `{ className: "a" }` or
 * `{ value: "a", label: "A" }`, so props built before a JSX spread are
 * handled like attributes. Looks through callbacks inside the value and
 * stops at JSX.
 */
function isInCodeProperty(node) {
	for (
		let parent = node.parent;
		parent &&
		!ts.isJsxElement(parent) &&
		!ts.isJsxSelfClosingElement(parent) &&
		!ts.isJsxFragment(parent) &&
		!ts.isSourceFile(parent);
		parent = parent.parent
	) {
		if (
			ts.isPropertyAssignment(parent) &&
			isCodeAttribute({
				name: staticPropertyName(parent.name),
				intrinsic: false,
			})
		) {
			return true;
		}
	}
	return false;
}

// Functions and constructors whose string arguments are code: patterns,
// selectors, storage keys, URLs, event names, and attribute names.
const CODE_ARGUMENT_CALLS = new Set([
	"CustomEvent",
	"Event",
	"RegExp",
	"URL",
	"URLSearchParams",
	"addEventListener",
	"closest",
	"dispatchEvent",
	"fetch",
	"getAttribute",
	"getElementById",
	"getElementsByClassName",
	"getElementsByTagName",
	"getItem",
	"hasAttribute",
	"matches",
	"querySelector",
	"querySelectorAll",
	"removeAttribute",
	"removeEventListener",
	"removeItem",
	"setAttribute",
	"setItem",
]);

/** Returns the name a call invokes: `f` in `f()` and `a.f()`, or "". */
function calleeName(expression) {
	const callee = skipTransparent(expression);
	if (ts.isIdentifier(callee)) {
		return callee.text;
	}
	if (ts.isPropertyAccessExpression(callee)) {
		return callee.name.text;
	}
	return "";
}

/** Returns true when the literal is code rather than displayed text. */
function isNonDisplayLiteral(literal) {
	const attribute = enclosingJsxAttribute(literal);
	if ((attribute && isCodeAttribute(attribute)) || isInCodeProperty(literal)) {
		return true;
	}
	// Look through wrappers such as `("Task Events" as const)` to the node
	// that uses the value.
	let node = literal;
	let parent = literal.parent;
	while (isTransparent(parent)) {
		node = parent;
		parent = parent.parent;
	}
	if (
		ts.isImportDeclaration(parent) ||
		ts.isExportDeclaration(parent) ||
		ts.isExternalModuleReference(parent) ||
		ts.isLiteralTypeNode(parent) ||
		ts.isCaseClause(parent) ||
		ts.isEnumMember(parent) ||
		ts.isComputedPropertyName(parent) ||
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
	if (
		(ts.isCallExpression(parent) || ts.isNewExpression(parent)) &&
		parent.arguments?.includes(node) &&
		CODE_ARGUMENT_CALLS.has(calleeName(parent.expression))
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

// Named HTML entities that JSX decodes, as "name codePoint" pairs: the
// HTML 4 set that TypeScript's JSX transform supports. JSX renders
// entities in text and attribute strings, so they are decoded before
// words are classified: "Validate&hellip;" reads as "Validate…".
const NAMED_ENTITIES = new Map(
	[
		"quot 34 amp 38 apos 39 lt 60 gt 62 nbsp 160 iexcl 161 cent 162",
		"pound 163 curren 164 yen 165 brvbar 166 sect 167 uml 168 copy 169",
		"ordf 170 laquo 171 not 172 shy 173 reg 174 macr 175 deg 176",
		"plusmn 177 sup2 178 sup3 179 acute 180 micro 181 para 182",
		"middot 183 cedil 184 sup1 185 ordm 186 raquo 187 frac14 188",
		"frac12 189 frac34 190 iquest 191 Agrave 192 Aacute 193 Acirc 194",
		"Atilde 195 Auml 196 Aring 197 AElig 198 Ccedil 199 Egrave 200",
		"Eacute 201 Ecirc 202 Euml 203 Igrave 204 Iacute 205 Icirc 206",
		"Iuml 207 ETH 208 Ntilde 209 Ograve 210 Oacute 211 Ocirc 212",
		"Otilde 213 Ouml 214 times 215 Oslash 216 Ugrave 217 Uacute 218",
		"Ucirc 219 Uuml 220 Yacute 221 THORN 222 szlig 223 agrave 224",
		"aacute 225 acirc 226 atilde 227 auml 228 aring 229 aelig 230",
		"ccedil 231 egrave 232 eacute 233 ecirc 234 euml 235 igrave 236",
		"iacute 237 icirc 238 iuml 239 eth 240 ntilde 241 ograve 242",
		"oacute 243 ocirc 244 otilde 245 ouml 246 divide 247 oslash 248",
		"ugrave 249 uacute 250 ucirc 251 uuml 252 yacute 253 thorn 254",
		"yuml 255 OElig 338 oelig 339 Scaron 352 scaron 353 Yuml 376",
		"fnof 402 circ 710 tilde 732 Alpha 913 Beta 914 Gamma 915 Delta 916",
		"Epsilon 917 Zeta 918 Eta 919 Theta 920 Iota 921 Kappa 922",
		"Lambda 923 Mu 924 Nu 925 Xi 926 Omicron 927 Pi 928 Rho 929",
		"Sigma 931 Tau 932 Upsilon 933 Phi 934 Chi 935 Psi 936 Omega 937",
		"alpha 945 beta 946 gamma 947 delta 948 epsilon 949 zeta 950",
		"eta 951 theta 952 iota 953 kappa 954 lambda 955 mu 956 nu 957",
		"xi 958 omicron 959 pi 960 rho 961 sigmaf 962 sigma 963 tau 964",
		"upsilon 965 phi 966 chi 967 psi 968 omega 969 thetasym 977",
		"upsih 978 piv 982 ensp 8194 emsp 8195 thinsp 8201 zwnj 8204",
		"zwj 8205 lrm 8206 rlm 8207 ndash 8211 mdash 8212 lsquo 8216",
		"rsquo 8217 sbquo 8218 ldquo 8220 rdquo 8221 bdquo 8222 dagger 8224",
		"Dagger 8225 bull 8226 hellip 8230 permil 8240 prime 8242",
		"Prime 8243 lsaquo 8249 rsaquo 8250 oline 8254 frasl 8260 euro 8364",
		"image 8465 weierp 8472 real 8476 trade 8482 alefsym 8501 larr 8592",
		"uarr 8593 rarr 8594 darr 8595 harr 8596 crarr 8629 lArr 8656",
		"uArr 8657 rArr 8658 dArr 8659 hArr 8660 forall 8704 part 8706",
		"exist 8707 empty 8709 nabla 8711 isin 8712 notin 8713 ni 8715",
		"prod 8719 sum 8721 minus 8722 lowast 8727 radic 8730 prop 8733",
		"infin 8734 ang 8736 and 8743 or 8744 cap 8745 cup 8746 int 8747",
		"there4 8756 sim 8764 cong 8773 asymp 8776 ne 8800 equiv 8801",
		"le 8804 ge 8805 sub 8834 sup 8835 nsub 8836 sube 8838 supe 8839",
		"oplus 8853 otimes 8855 perp 8869 sdot 8901 lceil 8968 rceil 8969",
		"lfloor 8970 rfloor 8971 lang 9001 rang 9002 loz 9674 spades 9824",
		"clubs 9827 hearts 9829 diams 9830",
	]
		.join(" ")
		.match(/\S+ \d+/g)
		.map((pair) => {
			const [name, codePoint] = pair.split(" ");
			return [name, String.fromCodePoint(Number(codePoint))];
		}),
);

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
		return NAMED_ENTITIES.get(name) ?? null;
	}
	if (codePoint > 0x10ffff) {
		return null;
	}
	const char = String.fromCodePoint(codePoint);
	// Never let source text pose as a substitution.
	return char === SUBSTITUTION ? "\uFFFD" : char;
}

/**
 * Returns the known HTML entity at `position` in `sourceText` as its
 * decoded text and source length, or null.
 */
function entityAt(sourceText, position) {
	if (sourceText[position] !== "&") {
		return null;
	}
	ENTITY.lastIndex = position;
	const match = ENTITY.exec(sourceText);
	const decoded = match && decodeEntity(match);
	return decoded ? { decoded, length: match[0].length } : null;
}

const ESCAPE =
	/\\(?:u\{([\da-fA-F]+)\}|u([\da-fA-F]{4})|x([\da-fA-F]{2})|(\r\n|[\s\S]))/y;

// Single-character escapes that do not stand for themselves.
const SIMPLE_ESCAPES = {
	0: "\0",
	b: "\b",
	f: "\f",
	n: "\n",
	r: "\r",
	t: "\t",
	v: "\v",
};

/**
 * Returns the JavaScript string escape at `position` in `sourceText` as
 * its decoded text and source length, or null.
 */
function escapeAt(sourceText, position) {
	if (sourceText[position] !== "\\") {
		return null;
	}
	ESCAPE.lastIndex = position;
	const match = ESCAPE.exec(sourceText);
	if (!match) {
		return null;
	}
	const [escape, braced, unicode, hex, char] = match;
	const codePoint = Number.parseInt(braced ?? unicode ?? hex ?? "", 16);
	let decoded;
	if (!Number.isNaN(codePoint)) {
		if (codePoint > 0x10ffff) {
			return null;
		}
		decoded = String.fromCodePoint(codePoint);
		// An escaped surrogate pair, such as "\\uD801\\uDC00", is one
		// character.
		const low =
			codePoint >= 0xd800 &&
			codePoint <= 0xdbff &&
			escapeAt(sourceText, position + escape.length);
		if (low && /^[\uDC00-\uDFFF]$/.test(low.decoded)) {
			return {
				decoded: decoded + low.decoded,
				length: escape.length + low.length,
			};
		}
	} else if (/^[\r\n\u2028\u2029]/.test(char)) {
		// A line continuation renders nothing.
		decoded = "";
	} else {
		decoded = SIMPLE_ESCAPES[char] ?? char;
	}
	// Never let source text pose as a substitution.
	if (decoded === SUBSTITUTION) {
		decoded = "\uFFFD";
	}
	return { decoded, length: escape.length };
}

/**
 * Returns the encoded character at `position` as its decoded text and
 * source length, or null. `encoding` is "jsx" for HTML entities, as JSX
 * renders them in text and attribute strings, and "js" for JavaScript
 * string escapes.
 */
function encodedAt(sourceText, position, encoding) {
	if (encoding === "jsx") {
		return entityAt(sourceText, position);
	}
	if (encoding === "js") {
		return escapeAt(sourceText, position);
	}
	return null;
}

/**
 * Appends the source text in [start, end) to `target`, recording the
 * source position of each character. Characters encoded per `encoding`
 * (see encodedAt) are replaced by the text they render, each mapped to
 * the position where the encoding starts.
 */
function appendSource(target, sourceText, start, end, encoding) {
	let position = start;
	while (position < end) {
		const encoded = encodedAt(sourceText, position, encoding);
		if (encoded && position + encoded.length <= end) {
			target.text += encoded.decoded;
			for (let i = 0; i < encoded.decoded.length; i++) {
				target.positions.push(position);
			}
			position += encoded.length;
			continue;
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

// Elements whose text is code or keyboard input rather than prose. They
// are read as unknown text inside the surrounding phrase.
const CODE_ELEMENTS = new Set(["code", "kbd", "pre", "samp"]);

// Inline HTML elements: the phrasing content that renders text, except
// form controls such as button, whose text is a label of its own.
// Adjacent inline siblings render as one run of text, so a phrase
// continues across them when an edge renders a space. Other intrinsic
// elements render as blocks.
const INLINE_ELEMENTS = new Set([
	"a",
	"abbr",
	"b",
	"bdi",
	"bdo",
	"cite",
	"data",
	"del",
	"dfn",
	"em",
	"i",
	"ins",
	"label",
	"mark",
	"q",
	"rp",
	"rt",
	"ruby",
	"s",
	"small",
	"span",
	"strong",
	"sub",
	"sup",
	"time",
	"u",
	"var",
]);

/** Returns the tag name of an intrinsic element, such as "span". */
function intrinsicTagName(node) {
	if (!ts.isJsxElement(node)) {
		return null;
	}
	const tagName = node.openingElement.tagName;
	return ts.isIdentifier(tagName) && /^[a-z]/.test(tagName.text)
		? tagName.text
		: null;
}

// Caps the rendered variants checked for one JSX element or string. Past
// the cap, further conditional values are treated as substitutions and
// their text is checked on its own.
const MAX_VARIANTS = 64;

/**
 * Returns true for an expression that renders its operand unchanged:
 * parentheses and type-only wrappers such as `as const`, `satisfies`,
 * and `!`.
 */
function isTransparent(node) {
	return (
		ts.isParenthesizedExpression(node) ||
		ts.isAsExpression(node) ||
		ts.isSatisfiesExpression(node) ||
		ts.isNonNullExpression(node) ||
		ts.isTypeAssertionExpression(node)
	);
}

/** Returns the expression inside any transparent wrappers. */
function skipTransparent(node) {
	let inner = node;
	while (isTransparent(inner)) {
		inner = inner.expression;
	}
	return inner;
}

/**
 * Returns what an inline JSX expression may render: a JSX node, string,
 * template literal, string concatenation, or array to read as text, null
 * for nothing, or "substitution" for an unknown value.
 * `cond && <b>Text</b>` renders the element or nothing, `value || "Text"`
 * and `value ?? "Text"` render the value or the fallback, and a
 * conditional renders either branch.
 */
function renderedOptions(expression) {
	const node = skipTransparent(expression);
	if (
		ts.isJsxElement(node) ||
		ts.isJsxFragment(node) ||
		ts.isJsxSelfClosingElement(node) ||
		ts.isStringLiteral(node) ||
		ts.isNoSubstitutionTemplateLiteral(node) ||
		ts.isTemplateExpression(node) ||
		isStringConcatenation(node) ||
		ts.isArrayLiteralExpression(node)
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
 * of each literal part, without its `, ${, and } delimiters, and the
 * substituted expression between each pair of parts.
 */
function templateParts(node, sourceFile) {
	const head = node.head;
	const parts = [{ start: head.getStart(sourceFile) + 1, end: head.end - 2 }];
	for (const span of node.templateSpans) {
		parts.push({ substitution: span.expression });
		const literal = span.literal;
		const isTail = literal.kind === ts.SyntaxKind.TemplateTail;
		parts.push({
			start: literal.getStart(sourceFile) + 1,
			end: literal.end - (isTail ? 1 : 2),
		});
	}
	return parts;
}

/** Returns true for a string literal or template literal. */
function isStringNode(node) {
	return (
		ts.isStringLiteral(node) ||
		ts.isNoSubstitutionTemplateLiteral(node) ||
		ts.isTemplateExpression(node)
	);
}

function isPlus(node) {
	return (
		ts.isBinaryExpression(node) &&
		node.operatorToken.kind === ts.SyntaxKind.PlusToken
	);
}

/** Returns the operands of a chain of `+` expressions, in order. */
function concatenationOperands(node) {
	const operands = [];
	const add = (operand) => {
		const inner = skipTransparent(operand);
		if (isPlus(inner)) {
			add(inner.left);
			add(inner.right);
		} else {
			operands.push(inner);
		}
	};
	add(node);
	return operands;
}

/** Returns true for a `+` expression that joins at least one string. */
function isStringConcatenation(node) {
	return isPlus(node) && concatenationOperands(node).some(isStringNode);
}

/**
 * Returns true for the outermost `+` expression of a string
 * concatenation, so the inner `+` expressions are not read again.
 */
function isOutermostConcatenation(node) {
	let parent = node.parent;
	while (isTransparent(parent)) {
		parent = parent.parent;
	}
	return isStringConcatenation(node) && !isPlus(parent);
}

/**
 * Returns the parts of a string concatenation in order, like
 * templateParts: the source range of each string's text, and each other
 * operand as a substituted expression. `"New " + name` renders as one
 * phrase. The strings read here are added to `consumed`.
 */
function concatenationParts(node, sourceFile, consumed) {
	const parts = [];
	for (const operand of concatenationOperands(node)) {
		if (ts.isTemplateExpression(operand)) {
			consumed.add(operand);
			parts.push(...templateParts(operand, sourceFile));
		} else if (isStringNode(operand)) {
			consumed.add(operand);
			parts.push({
				start: operand.getStart(sourceFile) + 1,
				end: operand.end - 1,
			});
		} else {
			parts.push({ substitution: operand });
		}
	}
	return parts;
}

/**
 * Returns functions that build the rendered pieces of JSX and string
 * expressions: text ranges, substitutions for unknown values, boundaries
 * at the edges of nested elements, breaks for self-closing elements, and
 * choices between the pieces of each variant a conditional expression
 * renders. Nodes read here are added to `consumed` so they are not
 * checked again on their own.
 */
function createPieceBuilder(sourceFile, consumed) {
	const sourceText = sourceFile.text;
	let variants = 1;

	// `inline` marks pieces inside a string, where a substitution joins the
	// text around it, as in `${count}s`.
	const addNode = (node, pieces, inline = false) => {
		if (node === null) {
			return;
		}
		if (node === "substitution") {
			pieces.push({ kind: "substitution", inline });
		} else if (ts.isJsxText(node)) {
			consumed.add(node);
			const text = sourceText.slice(node.pos, node.end);
			// Whitespace with a line break between tags renders nothing.
			if (!/^\s*$/.test(text) || rendersSpace(text)) {
				pieces.push({
					kind: "text",
					start: node.pos,
					end: node.end,
					encoding: "jsx",
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
				encoding: "js",
			});
		} else if (ts.isTemplateExpression(node) || isPlus(node)) {
			consumed.add(node);
			const parts = isPlus(node)
				? concatenationParts(node, sourceFile, consumed)
				: templateParts(node, sourceFile);
			for (const { start, end, substitution } of parts) {
				if (substitution) {
					addExpression(substitution, pieces, true);
				} else {
					pieces.push({ kind: "text", start, end, encoding: "js" });
				}
			}
		} else if (ts.isArrayLiteralExpression(node)) {
			// React renders an array's items one after another, including
			// the items of a spread array literal.
			for (const element of node.elements) {
				const spread =
					ts.isSpreadElement(element) && skipTransparent(element.expression);
				if (spread && ts.isArrayLiteralExpression(spread)) {
					addNode(spread, pieces, inline);
				} else {
					addExpression(element, pieces, inline);
				}
			}
		} else if (ts.isJsxExpression(node)) {
			// An empty expression, such as {/* note */}, renders nothing.
			if (node.expression) {
				addExpression(node.expression, pieces);
			}
		} else if (CODE_ELEMENTS.has(intrinsicTagName(node))) {
			// Skip the rendered content, including text in expressions and
			// nested elements. Attributes, such as aria-label on the element
			// or on elements inside it, are still checked on their own.
			const consume = (child) => {
				if (ts.isJsxSelfClosingElement(child)) {
					return;
				}
				consumed.add(child);
				if (ts.isJsxElement(child) || ts.isJsxFragment(child)) {
					child.children.forEach(consume);
				} else {
					ts.forEachChild(child, consume);
				}
			};
			consume(node);
			// A pre element renders as a block; the others are inline.
			pieces.push(
				intrinsicTagName(node) === "pre"
					? { kind: "break" }
					: { kind: "substitution", inline },
			);
		} else if (ts.isJsxElement(node) || ts.isJsxFragment(node)) {
			consumed.add(node);
			const tagName = intrinsicTagName(node);
			const inlineElement =
				ts.isJsxFragment(node) || INLINE_ELEMENTS.has(tagName);
			// An intrinsic element that is not inline, such as a heading or
			// paragraph, renders as its own block. Components may render
			// either, so the space at their edges decides.
			const blockElement = tagName !== null && !inlineElement;
			const boundary = { kind: "boundary", inlineElement, blockElement };
			pieces.push({ ...boundary, edge: "open" });
			for (const child of node.children) {
				addNode(child, pieces);
			}
			pieces.push({ ...boundary, edge: "close" });
		} else {
			// A self-closing element, such as an icon, sits between
			// separate labels more often than inside a phrase.
			pieces.push({ kind: "break" });
		}
	};

	const addExpression = (expression, pieces, inline = false) => {
		const options = renderedOptions(expression);
		if (options.length === 1) {
			addNode(options[0], pieces, inline);
		} else if (
			options.every((option) => option === "substitution") ||
			variants * options.length > MAX_VARIANTS
		) {
			pieces.push({ kind: "substitution", inline });
		} else {
			variants *= options.length;
			pieces.push({
				kind: "choice",
				options: options.map((option) => {
					const optionPieces = [];
					addNode(option, optionPieces, inline);
					return optionPieces;
				}),
			});
		}
	};

	// Returns the phrases of every rendered variant of `pieces`.
	const join = (pieces) =>
		expandChoices(pieces).flatMap((sequence) =>
			joinJsxPieces(sequence, sourceText),
		);

	return { addNode, join };
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
 * adjacent elements are usually separate labels.
 */
function collectJsxTexts(container, sourceFile, consumed) {
	const builder = createPieceBuilder(sourceFile, consumed);
	const pieces = [];
	if (CODE_ELEMENTS.has(intrinsicTagName(container))) {
		// Consumes the code element without reading its text.
		builder.addNode(container, pieces);
		return [];
	}
	for (const child of container.children) {
		builder.addNode(child, pieces);
	}
	return builder.join(pieces);
}

/**
 * Collects the rendered text of a template literal or string
 * concatenation. Substituted values that render known strings, such as
 * `"Create " + (ok ? "New Workspace" : name)`, are read in place once per
 * variant.
 */
function collectStringTexts(node, sourceFile, consumed) {
	const builder = createPieceBuilder(sourceFile, consumed);
	const pieces = [];
	builder.addNode(node, pieces);
	return builder.join(pieces);
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
		if (rendersSpace(whitespace)) {
			return true;
		}
		// Past any dropped whitespace, an encoded space such as &nbsp;
		// still renders.
		const decoded = { text: "", positions: [] };
		appendSource(
			decoded,
			sourceText,
			edge === "end" ? piece.start : piece.start + whitespace.length,
			edge === "end" ? piece.end - whitespace.length : piece.end,
			piece.encoding,
		);
		return (edge === "end" ? /\s$/ : /^\s/).test(decoded.text);
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
			pieces[i - 1]?.edge === "close" &&
			!(piece.inlineElement && pieces[i - 1].inlineElement)
		) {
			// Sibling elements with nothing rendered between them are
			// separate labels, whatever whitespace they hold inside,
			// unless both are inline elements in one run of text.
			endPhrase();
		} else if (piece.kind === "boundary" && piece.blockElement) {
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
			appendSource(current, sourceText, piece.start, piece.end, piece.encoding);
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
	const addRange = (start, end, encoding) => {
		const text = { text: "", positions: [] };
		appendSource(text, sourceText, start, end, encoding);
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
					// JSX attribute strings decode HTML entities, not escapes.
					addRange(
						node.getStart(sourceFile),
						node.end,
						ts.isJsxAttribute(node.parent) ? "jsx" : "js",
					);
				}
				break;
			case ts.SyntaxKind.BinaryExpression:
			case ts.SyntaxKind.TemplateExpression: {
				const isString =
					ts.isTemplateExpression(node) || isOutermostConcatenation(node);
				if (!isString || consumed.has(node)) {
					break;
				}
				if (isNonDisplayLiteral(node)) {
					// Skip the whole value, including the strings inside it.
					return;
				}
				texts.push(...collectStringTexts(node, sourceFile, consumed));
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
		// A word that starts with an entity or escape, such as
		// "&#87;orkspace" or "\\u0057orkspace", maps to its first character;
		// replace the whole encoding with the lowercase letter.
		const entity =
			entityAt(sourceText, position) ?? escapeAt(sourceText, position);
		const initial = entity
			? entity.decoded
			: String.fromCodePoint(sourceText.codePointAt(position));
		const length = entity ? entity.length : initial.length;
		fixedText =
			fixedText.slice(0, position) +
			initial.toLowerCase() +
			fixedText.slice(position + length);
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
