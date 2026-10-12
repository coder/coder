import fs from "node:fs";
import path from "node:path";
import { baseModeFor, CONCRETE_THEMES } from ".";
import { migrateLegacyPreference, resolveActiveThemeName } from "./themeMode";

type FirstPaintRule = {
	media: "light" | "dark" | null;
	selector: string;
	mode: "light" | "dark";
};

// Reads the first-paint rules from index.css in source order. They all share
// one specificity, so the last matching rule decides the theme.
function readFirstPaintRules(): FirstPaintRule[] {
	const css = fs
		.readFileSync(path.resolve(__dirname, "../index.css"), "utf8")
		.replace(/\/\*[\s\S]*?\*\//g, "");
	const start = css.indexOf("html:not(.light, .dark) {");
	const rules: FirstPaintRule[] = [];
	const media: Array<FirstPaintRule["media"]> = [];
	let prelude = "";
	for (let i = start; i < css.length; i++) {
		const char = css[i];
		if (char === "{") {
			const head = prelude.trim();
			prelude = "";
			const mediaMatch = head.match(
				/^@media \(prefers-color-scheme: (light|dark)\)$/,
			);
			if (mediaMatch?.[1] === "light" || mediaMatch?.[1] === "dark") {
				media.push(mediaMatch[1]);
				continue;
			}
			const end = css.indexOf("}", i);
			const apply = css.slice(i, end).match(/@apply theme-(light|dark)-vars;/);
			if (!head.startsWith("html:not(.light, .dark)") || !apply) {
				break;
			}
			rules.push({
				media: media.at(-1) ?? null,
				selector: head,
				mode: apply[1] === "light" ? "light" : "dark",
			});
			i = end;
		} else if (char === "}") {
			if (media.pop() === undefined) {
				break;
			}
			prelude = "";
		} else {
			prelude += char;
		}
	}
	return rules;
}

const modes = ["", "single", "sync", "bogus"];
const preferences = [
	"",
	...CONCRETE_THEMES,
	"auto",
	"auto-protan-deuter",
	"auto-tritan",
	"lightbogus",
];
const slots = ["", ...CONCRETE_THEMES, "bogus"];
const osSchemes = ["light", "dark"] as const;

describe("first-paint theme rules in index.css", () => {
	const rules = readFirstPaintRules();
	const root = document.documentElement;

	afterEach(() => {
		for (const name of ["mode", "preference", "light", "dark"]) {
			root.removeAttribute(`data-theme-${name}`);
		}
	});

	it("are found in index.css", () => {
		expect(rules.map(({ media, mode }) => [media, mode])).toEqual([
			[null, "dark"],
			[null, "light"],
			["light", "light"],
			["dark", "light"],
		]);
	});

	it("pick the same base mode as ThemeProvider for every appearance setting", () => {
		const mismatches: string[] = [];
		for (const mode of modes) {
			for (const preference of preferences) {
				for (const light of slots) {
					for (const dark of slots) {
						root.setAttribute("data-theme-mode", mode);
						root.setAttribute("data-theme-preference", preference);
						root.setAttribute("data-theme-light", light);
						root.setAttribute("data-theme-dark", dark);
						const settings = {
							theme_mode: mode,
							theme_preference: preference,
							theme_light: light,
							theme_dark: dark,
						};
						for (const os of osSchemes) {
							const expected = baseModeFor(
								resolveActiveThemeName(migrateLegacyPreference(settings), os),
							);
							let actual: FirstPaintRule["mode"] | undefined;
							for (const rule of rules) {
								if ((rule.media ?? os) === os && root.matches(rule.selector)) {
									actual = rule.mode;
								}
							}
							if (actual !== expected) {
								mismatches.push(
									`${JSON.stringify({ ...settings, os })}: css=${actual} ThemeProvider=${expected}`,
								);
							}
						}
					}
				}
			}
		}
		expect(mismatches).toEqual([]);
	});
});
