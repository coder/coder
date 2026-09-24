// Custom oxlint rules for the Coder frontend, loaded through `jsPlugins` in
// .oxlintrc.jsonc.

// Cheap text prefilter checked before oxlint deserializes a file's AST. It must
// match every file the AST check could flag (false positives are fine): a
// type-only import from "react" always has `type` between `import` and
// `from "react"` within a single statement. Skipping files here avoids AST
// deserialization, which dominates the cost of a JS plugin rule.
const maybeReactTypeImport =
	/\bimport\b[^;]*?\btype\b[^;]*?\bfrom\s*["']react["']/;

const importedName = (spec) =>
	spec.imported.type === "Identifier" ? spec.imported.name : spec.imported.value;

/**
 * Flags type-only named imports from "react" and rewrites their references to
 * the global `React` namespace provided by @types/react, e.g. `FC` becomes
 * `React.FC`.
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
	// `createOnce` (oxlint-specific) enables the `before` hook, which can skip
	// a file entirely by returning false.
	createOnce(context) {
		return {
			before() {
				return maybeReactTypeImport.test(context.sourceCode.text);
			},
			ImportDeclaration(node) {
				if (node.source.value !== "react") return;
				const sourceCode = context.sourceCode;
				const declIsType = node.importKind === "type";
				const typeSpecs = node.specifiers.filter(
					(s) =>
						s.type === "ImportSpecifier" &&
						(declIsType || s.importKind === "type"),
				);
				if (typeSpecs.length === 0) return;

				const fix = (fixer) => {
					const fixes = [];
					for (const spec of typeSpecs) {
						for (const variable of sourceCode.getDeclaredVariables(spec)) {
							for (const ref of variable.references) {
								fixes.push(
									fixer.replaceText(
										ref.identifier,
										`React.${importedName(spec)}`,
									),
								);
							}
						}
					}
					const remaining = node.specifiers.filter(
						(s) => !typeSpecs.includes(s),
					);
					if (remaining.length === 0) {
						const end =
							sourceCode.text[node.range[1]] === "\n"
								? node.range[1] + 1
								: node.range[1];
						fixes.push(fixer.removeRange([node.range[0], end]));
						return fixes;
					}
					const parts = [];
					const def = remaining.find(
						(s) => s.type === "ImportDefaultSpecifier",
					);
					if (def) parts.push(sourceCode.getText(def));
					const named = remaining.filter((s) => s.type === "ImportSpecifier");
					if (named.length > 0) {
						parts.push(
							`{ ${named.map((s) => sourceCode.getText(s)).join(", ")} }`,
						);
					}
					fixes.push(
						fixer.replaceText(node, `import ${parts.join(", ")} from "react";`),
					);
					return fixes;
				};

				typeSpecs.forEach((spec, i) => {
					context.report({
						node: spec,
						messageId: "preferNamespace",
						data: { name: importedName(spec) },
						// Attach the whole-declaration fix to one report so fixes
						// for sibling specifiers do not overlap.
						fix: i === 0 ? fix : undefined,
					});
				});
			},
		};
	},
};

export default {
	meta: { name: "coder" },
	rules: { "prefer-react-namespace-types": preferReactNamespaceTypes },
};
