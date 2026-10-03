import fs from "node:fs";
import path from "node:path";
import { baseModeFor, CONCRETE_THEMES, type ConcreteThemeName } from ".";

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
	// alerts, badges, chips, and syntax highlighting. A concrete theme
	// must resolve every token after base mode and variant overrides are
	// applied.
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

const THEME_CLASSES = [
	":root",
	...CONCRETE_THEMES.map((themeName) => `.${themeName}`),
];

const COLORBLIND_THEME_CLASSES = [
	".dark-protan-deuter",
	".light-protan-deuter",
	".dark-tritan",
	".light-tritan",
];

const TRITAN_THEME_CLASSES = [".dark-tritan", ".light-tritan"];

function stripCssComments(css: string): string {
	return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

function extractBlock(css: string, selector: string): string | null {
	const cssWithoutComments = stripCssComments(css);
	for (const match of cssWithoutComments.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
		const selectorList = match[1];
		const block = match[2];
		if (selectorList === undefined || block === undefined) {
			continue;
		}

		const selectors = selectorList.split(",").map((value) => value.trim());
		if (selectors.includes(selector)) {
			return block;
		}
	}
	return null;
}

function extractVariable(block: string, variable: string): string | null {
	return extractVariables(block).get(variable) ?? null;
}

function extractVariables(block: string): Map<string, string> {
	const variables = new Map<string, string>();
	for (const match of block.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
		const variable = match[1];
		const value = match[2];
		if (variable === undefined || value === undefined) {
			continue;
		}
		variables.set(variable, value.trim());
	}
	return variables;
}

function extractEffectiveBlock(css: string, selector: string): string | null {
	const block = extractBlock(css, selector);
	if (block === null) {
		return null;
	}

	if (!selector.startsWith(".") || !selector.includes("-")) {
		return block;
	}

	const themeName = selector.slice(1) as ConcreteThemeName;
	const baseBlock = extractBlock(css, `.${baseModeFor(themeName)}`);
	if (baseBlock === null) {
		return null;
	}
	return `${baseBlock}\n${block}`;
}

describe("theme CSS variables", () => {
	const cssPath = path.resolve(__dirname, "../index.css");
	const css = fs.readFileSync(cssPath, "utf8");

	for (const selector of THEME_CLASSES) {
		describe(selector, () => {
			const block = extractBlock(css, selector);
			const effectiveBlock = extractEffectiveBlock(css, selector);

			it("has a rule block in index.css", () => {
				expect(block).not.toBeNull();
			});

			if (effectiveBlock !== null) {
				for (const variable of REQUIRED_VARIABLES) {
					it(`resolves ${variable}`, () => {
						expect(extractVariable(effectiveBlock, variable)).not.toBeNull();
					});
				}
			}
		});
	}

	for (const selector of COLORBLIND_THEME_CLASSES) {
		describe(`${selector} semantic separation`, () => {
			const block = extractEffectiveBlock(css, selector);

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
			const block = extractEffectiveBlock(css, selector);

			it("keeps warning surfaces on the fuchsia surface token", () => {
				expect(block).not.toBeNull();
				expect(extractVariable(block ?? "", "--surface-orange")).toBe(
					extractVariable(block ?? "", "--surface-magenta"),
				);
			});
		});
	}
});
