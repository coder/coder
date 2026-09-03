/**
 * Per-module placeholder text overrides for the template builder's
 * module configuration fields.
 *
 * The API delivers each module's variable defaults verbatim from its
 * `module.json`. Some of those defaults are prose that is more useful as
 * documentation than as placeholder text (for example the dotfiles
 * `description` default is a two-sentence explanation). This table lets
 * us substitute short, actionable placeholder hints for a specific set
 * of variables without changing the underlying module manifest.
 *
 * Lookup order used by the module configuration step:
 *   1. Override in this table (if present).
 *   2. Variable's `default` value.
 *   3. `Required` when the variable is required, otherwise empty.
 */
export const MODULE_FIELD_PLACEHOLDERS: Readonly<
	Record<string, Readonly<Record<string, string>>>
> = {
	codex: {
		model_reasoning_effort: "none, minimal, low, medium, high, or xhigh",
	},
	"claude-code": {
		anthropic_api_key: "sk-ant-...",
		model: "e.g. claude-sonnet-5",
	},
	"git-clone": {
		base_dir: "$HOME",
		branch_name: "Default branch",
	},
	dotfiles: {
		description:
			"e.g https://dotfiles.github.io or an SSH URL git@host:user/repo",
	},
};

/**
 * Returns the placeholder override for the given module and variable,
 * or `undefined` when no override exists.
 */
export function getModuleFieldPlaceholder(
	moduleId: string,
	variableName: string,
): string | undefined {
	return MODULE_FIELD_PLACEHOLDERS[moduleId]?.[variableName];
}
