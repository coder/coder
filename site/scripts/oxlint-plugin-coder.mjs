/**
 * Enforces `React.X` for React types instead of named type imports:
 *
 *   import type { FC } from "react";    // flagged
 *   const Foo: FC = () => null;
 *
 *   const Foo: React.FC = () => null;   // preferred, no import needed
 *
 * `React` is a global namespace declared by @types/react, so type positions
 * can use it without importing anything. Value imports such as `useState`
 * are left alone.
 */
const preferReactNamespaceTypes = {
	meta: {
		type: "suggestion",
		fixable: "code",
		messages: {
			preferNamespace:
				'Use React.{{name}} instead of importing the type from "react".',
		},
	},

	// oxlint-specific alternative to `create`. It lets the rule define a
	// `before` hook, which runs before the file's AST is built and can skip
	// the file by returning false.
	createOnce(context) {
		return {
			before() {
				return mightHaveReactTypeImport(context.sourceCode.text);
			},

			ImportDeclaration(declaration) {
				if (declaration.source.value !== "react") {
					return;
				}

				const typeSpecifiers = declaration.specifiers.filter((specifier) =>
					isTypeOnlySpecifier(declaration, specifier),
				);

				// One fix covers every type specifier in the declaration. They
				// all edit the same import statement, so separate fixes would
				// overlap and be discarded.
				const fixAll = (fixer) => [
					...typeSpecifiers.flatMap((specifier) =>
						replaceReferencesWithNamespace(
							fixer,
							context.sourceCode,
							specifier,
						),
					),
					removeTypeSpecifiers(
						fixer,
						context.sourceCode,
						declaration,
						typeSpecifiers,
					),
				];

				typeSpecifiers.forEach((specifier, index) => {
					context.report({
						node: specifier,
						messageId: "preferNamespace",
						data: { name: specifier.imported.name },
						fix: index === 0 ? fixAll : undefined,
					});
				});
			},
		};
	},
};

/**
 * Cheap text check that runs before oxlint builds the AST. Building and
 * walking the AST is most of the cost of a JS plugin rule, so skipping files
 * here keeps the rule fast.
 *
 * It must return true for every file the rule could flag (false positives are
 * fine). A type-only import from "react" always has `type` between `import`
 * and `from "react"` within one statement, which is what the regex matches.
 */
function mightHaveReactTypeImport(sourceText) {
	return /\bimport\b[^;]*?\btype\b[^;]*?\bfrom\s*["']react["']/.test(
		sourceText,
	);
}

/**
 * True for `FC` in both `import type { FC } from "react"` and
 * `import { type FC } from "react"`.
 */
function isTypeOnlySpecifier(declaration, specifier) {
	if (specifier.type !== "ImportSpecifier") {
		return false;
	}
	return declaration.importKind === "type" || specifier.importKind === "type";
}

/**
 * Uses scope analysis, so unrelated identifiers with the same name are
 * untouched. The replacement uses the name exported by "react", so aliases are
 * resolved: `KE` from `import { type KeyboardEvent as KE }` becomes
 * `React.KeyboardEvent`.
 */
function replaceReferencesWithNamespace(fixer, sourceCode, specifier) {
	const replacement = `React.${specifier.imported.name}`;
	return sourceCode
		.getDeclaredVariables(specifier)
		.flatMap((variable) => variable.references)
		.map((reference) => fixer.replaceText(reference.identifier, replacement));
}

function removeTypeSpecifiers(fixer, sourceCode, declaration, typeSpecifiers) {
	const remaining = declaration.specifiers.filter(
		(specifier) => !typeSpecifiers.includes(specifier),
	);

	if (remaining.length === 0) {
		const [start, end] = declaration.range;
		const endsWithNewline = sourceCode.text[end] === "\n";
		return fixer.removeRange([start, endsWithNewline ? end + 1 : end]);
	}

	return fixer.replaceText(
		declaration,
		buildReactImport(sourceCode, remaining),
	);
}

/**
 * Formatting is normalized, and comments inside the original braces are not
 * preserved.
 */
function buildReactImport(sourceCode, specifiers) {
	const defaultSpecifier = specifiers.find(
		(specifier) => specifier.type === "ImportDefaultSpecifier",
	);
	const namedSpecifiers = specifiers.filter(
		(specifier) => specifier.type === "ImportSpecifier",
	);

	const clauses = [];
	if (defaultSpecifier) {
		clauses.push(sourceCode.getText(defaultSpecifier));
	}
	if (namedSpecifiers.length > 0) {
		const names = namedSpecifiers.map((specifier) =>
			sourceCode.getText(specifier),
		);
		clauses.push(`{ ${names.join(", ")} }`);
	}

	return `import ${clauses.join(", ")} from "react";`;
}

export default {
	meta: { name: "coder" },
	rules: { "prefer-react-namespace-types": preferReactNamespaceTypes },
};
