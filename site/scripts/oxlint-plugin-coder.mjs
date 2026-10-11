// Local oxlint rules for patterns that no built-in oxlint or Biome rule
// covers. Loaded through jsPlugins in .oxlintrc.jsonc.

const DISABLE_DIRECTIVE = /^\s*(?:eslint|oxlint)-disable(?:-next-line|-line)?(?:\s|$)/;
const DIRECTIVE_REASON = /\s--\s*\S/;

const hasUncommentedEmptyBody = (node, sourceCode) =>
	(node.type === "ArrowFunctionExpression" ||
		node.type === "FunctionExpression") &&
	node.body.type === "BlockStatement" &&
	node.body.body.length === 0 &&
	sourceCode.getCommentsInside(node.body).length === 0;

const noEmptyCatchCallback = {
	meta: {
		type: "suggestion",
		docs: {
			description:
				"Disallow .catch() handlers with an empty body, which silently swallow rejections.",
		},
		messages: {
			emptyCatchCallback:
				"Do not swallow rejections with an empty .catch() handler. Handle the error, or explain in a comment inside the handler why it is safe to ignore.",
		},
		schema: [],
	},
	create(context) {
		return {
			CallExpression(node) {
				const { callee } = node;
				if (
					callee.type !== "MemberExpression" ||
					callee.computed ||
					callee.property.type !== "Identifier" ||
					callee.property.name !== "catch"
				) {
					return;
				}
				const [handler] = node.arguments;
				if (handler && hasUncommentedEmptyBody(handler, context.sourceCode)) {
					context.report({ node: handler, messageId: "emptyCatchCallback" });
				}
			},
		};
	},
};

const noAsUnknownAs = {
	meta: {
		type: "suggestion",
		docs: {
			description:
				"Disallow `as unknown as T` double assertions outside tests and test helpers.",
		},
		messages: {
			asUnknownAs:
				"Do not cast through `as unknown as`. Fix the types, narrow with a type guard, or validate the value.",
		},
		schema: [],
	},
	create(context) {
		return {
			TSAsExpression(node) {
				const inner = node.expression;
				if (
					inner.type === "TSAsExpression" &&
					inner.typeAnnotation.type === "TSUnknownKeyword"
				) {
					context.report({ node, messageId: "asUnknownAs" });
				}
			},
		};
	},
};

const requireDisableReason = {
	meta: {
		type: "suggestion",
		docs: {
			description:
				"Require a `-- reason` on every oxlint-disable and eslint-disable directive.",
		},
		messages: {
			missingReason:
				"Explain this lint suppression: append ` -- <reason>` to the directive.",
		},
		schema: [],
	},
	create(context) {
		return {
			Program() {
				for (const comment of context.sourceCode.getAllComments()) {
					if (
						DISABLE_DIRECTIVE.test(comment.value) &&
						!DIRECTIVE_REASON.test(comment.value)
					) {
						context.report({ loc: comment.loc, messageId: "missingReason" });
					}
				}
			},
		};
	},
};

export default {
	meta: { name: "coder" },
	rules: {
		"no-as-unknown-as": noAsUnknownAs,
		"no-empty-catch-callback": noEmptyCatchCallback,
		"require-disable-reason": requireDisableReason,
	},
};
