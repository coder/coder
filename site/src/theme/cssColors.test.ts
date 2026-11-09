import { installThemeStyles } from "#/testHelpers/themeStyles";
import { readThemeColors } from "./cssColors";
import { getMonacoTheme } from "./monaco";

vi.stubGlobal("jest", vi);
await import("jest-canvas-mock");

describe("native API theme colors", () => {
	beforeAll(installThemeStyles);
	beforeEach(() => {
		vi.spyOn(
			CanvasRenderingContext2D.prototype,
			"getImageData",
		).mockReturnValue(
			new ImageData(new Uint8ClampedArray([1, 16, 255, 255]), 1, 1),
		);
	});
	afterEach(() => vi.restoreAllMocks());

	it("returns CSS colors directly for APIs that support them", () => {
		const getContext = vi.spyOn(HTMLCanvasElement.prototype, "getContext");
		expect(readThemeColors("light", ["--confetti-primary"])).toEqual({
			"--confetti-primary": "oklch(58.8% 0.158 241.966)",
		});
		expect(getContext).not.toHaveBeenCalled();
	});

	it.each(["dark", "light"] as const)(
		"resolves %s Tailwind colors through canvas independently of the root theme",
		(mode) => {
			const paint = vi.spyOn(
				CanvasRenderingContext2D.prototype,
				"fillStyle",
				"set",
			);
			document.documentElement.classList.add(
				mode === "dark" ? "light" : "dark",
			);
			try {
				expect(readThemeColors(mode, ["--confetti-primary"], "hex")).toEqual({
					"--confetti-primary": "#0110ff",
				});
				expect(paint).toHaveBeenCalledWith(
					mode === "dark"
						? "oklch(68.5% 0.169 237.323)"
						: "oklch(58.8% 0.158 241.966)",
				);
			} finally {
				document.documentElement.classList.remove("dark", "light");
			}
		},
	);

	it.each(["dark", "light"] as const)(
		"provides concrete %s hex colors to Monaco",
		(mode) => {
			const theme = getMonacoTheme(mode);
			expect(theme.base).toBe(mode === "dark" ? "vs-dark" : "vs");
			expect(theme.colors).toEqual({
				"editor.foreground": "#0110ff",
				"editor.background": "#0110ff",
			});
			for (const rule of theme.rules) {
				expect(rule.foreground).toMatch(/^[0-9a-f]{6}$/i);
			}
			expect(theme.rules).toContainEqual({
				token: "comment",
				foreground: "6b737c",
			});
		},
	);
});
