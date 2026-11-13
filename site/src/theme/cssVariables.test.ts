import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
	baseModeFor,
	CONCRETE_THEMES,
	type ConcreteThemeName,
	DEFAULT_THEME,
} from ".";

// Use filesystem paths to avoid Vite's browser asset URL transform.
const moduleDir = path.dirname(fileURLToPath(import.meta.url));
const css = fs.readFileSync(path.resolve(moduleDir, "../index.css"), "utf8");
const html = fs.readFileSync(
	path.resolve(moduleDir, "../../index.html"),
	"utf8",
);
const manifest: { theme_color?: string; background_color?: string } =
	JSON.parse(
		fs.readFileSync(
			path.resolve(moduleDir, "../../static/manifest.json"),
			"utf8",
		),
	);

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
	"--surface-git-added-emphasis",
	"--surface-git-deleted-emphasis",
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

const THEME_VARIABLES = [
	...REQUIRED_VARIABLES,
	...["success", "error", "warning", "active", "inactive", "notice"].flatMap(
		(role) => [`--chart-${role}`, `--chart-${role}-border`],
	),
	"--confetti-primary",
	"--confetti-secondary",
	...[
		"foreground",
		"background",
		"comment",
		"type",
		"string",
		"variable",
		"brace",
	].map((token) => `--editor-${token}`),
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

function extractBlocks(css: string, selector: string): string[] {
	const blocks: string[] = [];
	const cssWithoutComments = stripCssComments(css);
	for (const match of cssWithoutComments.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
		const selectorList = match[1];
		const block = match[2];
		if (selectorList === undefined || block === undefined) {
			continue;
		}

		const selectors = selectorList.split(",").map((value) => value.trim());
		if (selectors.includes(selector)) {
			blocks.push(block);
		}
	}
	return blocks;
}

function extractBlock(css: string, selector: string): string | null {
	return extractBlocks(css, selector)[0] ?? null;
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
	describe(":root", () => {
		const block = extractBlock(css, ":root");

		it("has a rule block in index.css", () => {
			expect(block).not.toBeNull();
		});

		it.each(REQUIRED_VARIABLES)("resolves %s", (variable) => {
			expect(extractVariable(block ?? "", variable)).not.toBeNull();
		});
	});

	for (const themeName of CONCRETE_THEMES) {
		const selector = `.${themeName}`;
		describe(selector, () => {
			const block = extractBlock(css, selector);
			const effectiveBlock = extractEffectiveBlock(css, selector);

			it("has a rule block in index.css", () => {
				expect(block).not.toBeNull();
			});

			it.each(THEME_VARIABLES)("resolves %s", (variable) => {
				expect(extractVariable(effectiveBlock ?? "", variable)).not.toBeNull();
			});
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

	for (const name of CONCRETE_THEMES) {
		it(`${name} distinguishes chart success, failure, warning, canceled, and skipped states`, () => {
			const block = extractEffectiveBlock(css, `.${name}`) ?? "";
			const fills = ["success", "error", "warning", "notice", "inactive"].map(
				(role) => extractVariable(block, `--chart-${role}`),
			);
			expect(new Set(fills).size).toBe(fills.length);
		});
	}

	it.each(CONCRETE_THEMES.filter((name) => name !== baseModeFor(name)))(
		"%s adapts chart fills and borders for colorblind users",
		(name) => {
			const variant = extractEffectiveBlock(css, `.${name}`) ?? "";
			const base = extractEffectiveBlock(css, `.${baseModeFor(name)}`) ?? "";
			for (const role of ["success", "error", "warning"]) {
				for (const variable of [`--chart-${role}`, `--chart-${role}-border`]) {
					expect(extractVariable(variant, variable), variable).not.toBe(
						extractVariable(base, variable),
					);
				}
			}
		},
	);

	it("gives body the surface that ThemeProvider copies into theme-color", () => {
		expect(extractBlocks(css, "body").join("\n")).toContain(
			"bg-surface-primary",
		);
	});

	// The index.html metas color the title bar until ThemeProvider overwrites
	// them, and the manifest colors apply before the page loads, so each must
	// be the surface of the theme it stands in for.
	describe("static theme colors", () => {
		const toRGB = (source: string, color: string | null | undefined) => {
			const probe = document.createElement("div");
			probe.style.color = color ?? "";
			if (!probe.style.color) {
				throw new Error(`${source} is not a CSS color: ${color}`);
			}
			return probe.style.color;
		};
		const surfaceOf = (selector: string) => {
			const block = extractBlock(css, selector) ?? "";
			return toRGB(
				`--surface-primary in the index.css ${selector} block`,
				`hsl(${extractVariable(block, "--surface-primary")})`,
			);
		};

		it("gives each OS color scheme its theme's surface in index.html", () => {
			const metas = Array.from(
				new DOMParser()
					.parseFromString(html, "text/html")
					.getElementsByName("theme-color"),
			);
			const colorFor = (scheme: "light" | "dark") =>
				metas
					.find(
						(meta) =>
							meta.getAttribute("media") ===
							`(prefers-color-scheme: ${scheme})`,
					)
					?.getAttribute("content");

			expect(metas).toHaveLength(2);
			expect(toRGB("index.html light theme-color", colorFor("light"))).toBe(
				surfaceOf(".light"),
			);
			expect(toRGB("index.html dark theme-color", colorFor("dark"))).toBe(
				surfaceOf(".dark"),
			);
		});

		it("uses the default theme's surface in the PWA manifest", () => {
			const surface = surfaceOf(`.${DEFAULT_THEME}`);

			expect(toRGB("manifest.json theme_color", manifest.theme_color)).toBe(
				surface,
			);
			expect(
				toRGB("manifest.json background_color", manifest.background_color),
			).toBe(surface);
		});
	});
});
