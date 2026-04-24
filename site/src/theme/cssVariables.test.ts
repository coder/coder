import fs from "node:fs";
import path from "node:path";

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
	"--git-added-bright",
	"--git-deleted-bright",
	"--git-merged-bright",
];

// Every theme the user can actually end up rendered as. "auto*" resolves
// to one of these, so we only validate the concrete classes here.
const THEME_CLASSES = [
	":root",
	".light",
	".dark",
	".dark-protan-deuter",
	".light-protan-deuter",
	".dark-tritan",
	".light-tritan",
];

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
});
