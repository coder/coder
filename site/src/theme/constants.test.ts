import { terminalFonts } from "./constants";

describe("terminalFonts", () => {
	it("uses the terminal symbol fallback before generic monospace", () => {
		for (const fontFamily of Object.values(terminalFonts)) {
			expect(fontFamily).toMatch(/'Coder Terminal Symbols', monospace$/);
		}
	});
});
