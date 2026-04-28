import fs from "node:fs";
import path from "node:path";
import { CONCRETE_THEMES } from "./colorblind";

// These CSS variables drive the diff panel and the semantic color roles
// that are most affected by colorblindness. Every theme class block must
// override each one so switching themes does not leave stale values from
// a previous theme leaking through.
const REQUIRED_VARIABLES = [
	"--content-primary",
	"--content-success",
	"--content-destructive",
	"--content-warning",
	"--surface-primary",
	"--border-default",
	"--border-success",
	"--border-destructive",
	"--git-added",
	"--git-deleted",
	"--git-modified",
	"--git-merged",
	"--surface-git-added",
	"--surface-git-deleted",
	// Extended palette surface. These carry semantic color meaning in
	// alerts, badges, chips, and syntax highlighting. A theme block that
	// omits one would silently inherit the base value and break the
	// colorblind intent on the affected surface.
	"--content-link",
	"--surface-destructive",
	"--surface-green",
	"--surface-orange",
	"--surface-sky",
	"--surface-red",
	"--surface-purple",
	"--surface-magenta",
	"--surface-git-merged",
	"--border-warning",
	"--border-sky",
	"--border-green",
	"--border-magenta",
	"--border-purple",
	"--highlight-purple",
	"--highlight-green",
	"--highlight-orange",
	"--highlight-sky",
	"--highlight-red",
	"--highlight-magenta",
	"--syntax-key",
	"--syntax-string",
	"--syntax-number",
	"--syntax-boolean",
	"--git-added-bright",
	"--git-deleted-bright",
	"--git-merged-bright",
];

// Every theme the user can actually end up rendered as. "auto*" resolves
// to one of these, so we only validate the concrete classes here.
const THEME_CLASSES = [
	":root",
	...CONCRETE_THEMES.map((themeName) => `.${themeName}`),
];

const COLORBLIND_THEME_CLASSES = CONCRETE_THEMES.filter((themeName) =>
	themeName.includes("-"),
).map((themeName) => `.${themeName}`);

const TRITAN_THEME_CLASSES = CONCRETE_THEMES.filter((themeName) =>
	themeName.endsWith("-tritan"),
).map((themeName) => `.${themeName}`);

function extractBlock(css: string, selector: string): string | null {
	// Selectors can be grouped with commas in the stylesheet, for example
	// `:root, .light { ... }`. Match any occurrence where the selector is a
	// standalone token in the prelude so `.dark` does not match inside
	// `.dark-tritan`.
	const pattern = new RegExp(
		`(?:^|[,\\s])${escapeRegex(selector)}(?:[,\\s][^{]*)?\\s*\\{([^}]*)\\}`,
		"m",
	);
	const match = css.match(pattern);
	return match ? match[1] : null;
}

function escapeRegex(value: string): string {
	return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function extractVariable(block: string, variable: string): string | null {
	const match = block.match(
		new RegExp(`${escapeRegex(variable)}\\s*:\\s*([^;]+);`),
	);
	return match ? match[1].trim() : null;
}

describe("theme CSS variables", () => {
	const cssPath = path.resolve(__dirname, "../index.css");
	const css = fs.readFileSync(cssPath, "utf8");

	for (const selector of THEME_CLASSES) {
		describe(selector, () => {
			const block = extractBlock(css, selector);

			it("has a rule block in index.css", () => {
				expect(block).not.toBeNull();
			});

			if (block !== null) {
				for (const variable of REQUIRED_VARIABLES) {
					it(`defines ${variable}`, () => {
						expect(block).toMatch(
							new RegExp(`\\s${escapeRegex(variable)}\\s*:`),
						);
					});
				}
			}
		});
	}

	for (const selector of COLORBLIND_THEME_CLASSES) {
		describe(`${selector} semantic separation`, () => {
			const block = extractBlock(css, selector);

			it("keeps warning distinct from destructive colors", () => {
				expect(block).not.toBeNull();
				expect(extractVariable(block ?? "", "--content-warning")).not.toBe(
					extractVariable(block ?? "", "--content-destructive"),
				);
				expect(extractVariable(block ?? "", "--surface-orange")).not.toBe(
					extractVariable(block ?? "", "--surface-red"),
				);
			});

			it("keeps links distinct from success colors", () => {
				expect(block).not.toBeNull();
				expect(extractVariable(block ?? "", "--content-link")).not.toBe(
					extractVariable(block ?? "", "--content-success"),
				);
			});
		});
	}

	for (const selector of TRITAN_THEME_CLASSES) {
		describe(`${selector} warning surface`, () => {
			const block = extractBlock(css, selector);

			it("keeps warning surfaces on the fuchsia surface token", () => {
				expect(block).not.toBeNull();
				expect(extractVariable(block ?? "", "--surface-orange")).toBe(
					extractVariable(block ?? "", "--surface-magenta"),
				);
			});
		});
	}
});
