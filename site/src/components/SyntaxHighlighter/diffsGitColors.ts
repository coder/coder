/**
 * Diff addition and deletion colors from the dashboard theme, so
 * `@pierre/diffs` follows the active theme variant, including the colorblind
 * ones. Pair with `DIFFS_GIT_COLOR_CSS`.
 */
export const DIFFS_GIT_COLOR_STYLE = {
	"--diffs-addition-color-override": "hsl(var(--git-added))",
	"--diffs-deletion-color-override": "hsl(var(--git-deleted))",
	"--diffs-bg-addition-override": "hsl(var(--surface-git-added))",
	"--diffs-bg-deletion-override": "hsl(var(--surface-git-deleted))",
	"--diffs-bg-addition-number-override": "hsl(var(--surface-git-added))",
	"--diffs-bg-deletion-number-override": "hsl(var(--surface-git-deleted))",
	"--diffs-bg-addition-emphasis-override":
		"hsl(var(--surface-git-added-emphasis))",
	"--diffs-bg-deletion-emphasis-override":
		"hsl(var(--surface-git-deleted-emphasis))",
};

/**
 * Removes the blend `@pierre/diffs` applies between changed-line backgrounds
 * and the code background, so the `surface-git-*` tokens render as defined,
 * and keeps hover tinting changed lines like context lines. Relies on the
 * library's internal `--mix-*` variables (checked against 1.5.1).
 */
export const DIFFS_GIT_COLOR_CSS = [
	"[data-line-type='change-addition'], [data-line-type='change-deletion'] {",
	"  --mix-light: 0%;",
	"  --mix-dark: 0%;",
	"  --diffs-hover-mix-target: var(--diffs-mixer);",
	"}",
	// Line numbers keep a light blend so the gutter reads as its own column.
	"[data-line-type='change-addition']:is([data-column-number], [data-gutter-buffer]),",
	"[data-line-type='change-deletion']:is([data-column-number], [data-gutter-buffer]) {",
	"  --mix-light: 25%;",
	"  --mix-dark: 25%;",
	"}",
].join(" ");
