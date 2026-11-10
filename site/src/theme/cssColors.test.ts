import { installThemeStyles } from "#/testHelpers/themeStyles";
import { readThemeColors } from "./cssColors";
import { getMonacoTheme } from "./monaco";

describe("native API theme colors", () => {
	let style: HTMLStyleElement;

	beforeAll(installThemeStyles);
	beforeAll(async () => {
		vi.stubGlobal("jest", vi);
		await import("jest-canvas-mock");
	});
	beforeEach(() => {
		style = document.createElement("style");
		document.head.append(style);
		const pixels = new Map([
			["#000000", [0, 0, 0, 255]],
			["#ffffff", [255, 255, 255, 255]],
			["#0110ff", [1, 16, 255, 255]],
			["#0000ff", [0, 0, 255, 255]],
		]);
		vi.spyOn(
			CanvasRenderingContext2D.prototype,
			"getImageData",
		).mockImplementation(function (this: CanvasRenderingContext2D) {
			const color = this.fillStyle;
			const pixel = typeof color === "string" ? pixels.get(color) : undefined;
			if (!pixel) throw new Error(`Missing pixel fixture for ${color}`);
			return new ImageData(new Uint8ClampedArray(pixel), 1, 1);
		});
	});
	afterEach(() => {
		style.remove();
		vi.restoreAllMocks();
	});

	it.each([
		["#abc", "#aabbcc"],
		["#ABCD", "#AABBCCDD"],
		["#aBcDeF", "#aBcDeF"],
		["#123456aB", "#123456aB"],
		["black", "#000000"],
		["white", "#ffffff"],
		["rgb(1, 16, 255)", "#0110ff"],
		["hsl(240, 100%, 50%)", "#0000ff"],
	])("resolves %s to %s in hex format", (color, expected) => {
		style.textContent = `.dark { --test-color: ${color}; }`;
		expect(readThemeColors("dark", ["--test-color"], "hex")).toEqual({
			"--test-color": expected,
		});
	});

	it.each([
		"#ab",
		"#abcde",
		"#abcdef0",
		"#abcdef012",
		"#abg",
		"not-a-color",
		"prefix#abc",
		"#abcsuffix",
	])("rejects invalid color %s", (color) => {
		style.textContent = `.dark { --test-color: ${color}; }`;
		expect(() => readThemeColors("dark", ["--test-color"], "hex")).toThrow(
			"Invalid theme color --test-color for dark mode",
		);
	});

	it.each(["css", "hex"] as const)(
		"rejects missing theme colors in %s format",
		(format) => {
			expect(() =>
				readThemeColors("light", ["--missing-color"], format),
			).toThrow("Missing theme color --missing-color for light mode");
		},
	);

	it("rejects an invalid token instead of reusing the preceding color", () => {
		style.textContent =
			".dark { --valid-color: rgb(1, 16, 255); --invalid-color: invalid; }";
		expect(() =>
			readThemeColors("dark", ["--valid-color", "--invalid-color"], "hex"),
		).toThrow("Invalid theme color --invalid-color for dark mode");
	});

	it.each(["dark", "light"] as const)(
		"returns %s CSS colors directly, independently of the root theme",
		(mode) => {
			const getContext = vi.spyOn(HTMLCanvasElement.prototype, "getContext");
			document.documentElement.classList.add(
				mode === "dark" ? "light" : "dark",
			);
			try {
				expect(readThemeColors(mode, ["--confetti-primary"])).toEqual({
					"--confetti-primary":
						mode === "dark"
							? "oklch(68.5% 0.169 237.323)"
							: "oklch(58.8% 0.158 241.966)",
				});
				expect(getContext).not.toHaveBeenCalled();
			} finally {
				document.documentElement.classList.remove("dark", "light");
			}
		},
	);

	it.each(["dark", "light"] as const)(
		"provides concrete %s hex colors to Monaco",
		(mode) => {
			// The canvas mock cannot parse the theme's OKLCH colors.
			// Production CSS minification shortens the editor variable colors.
			style.textContent = `.${mode} {
				--editor-foreground: ${mode === "dark" ? "white" : "black"};
				--editor-background: ${mode === "dark" ? "black" : "white"};
				--editor-variable: ${mode === "dark" ? "#ddd" : "#444"};
			}`;
			const theme = getMonacoTheme(mode);
			expect(theme.base).toBe(mode === "dark" ? "vs-dark" : "vs");
			expect(theme.colors).toEqual({
				"editor.foreground": mode === "dark" ? "#ffffff" : "#000000",
				"editor.background": mode === "dark" ? "#000000" : "#ffffff",
			});
			for (const rule of theme.rules) {
				expect(rule.foreground).toMatch(/^[0-9a-f]{6}(?:[0-9a-f]{2})?$/i);
			}
			expect(theme.rules).toContainEqual({
				token: "comment",
				foreground: "6b737c",
			});
			expect(theme.rules).toContainEqual({
				token: "variable",
				foreground: mode === "dark" ? "dddddd" : "444444",
			});
		},
	);
});
