import { renderHook } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { usePreferredColorScheme } from "./usePreferredColorScheme";

const stubMatchMedia = (matches: boolean) => {
	const query: MediaQueryList = {
		matches,
		media: "(prefers-color-scheme: light)",
		onchange: null,
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
		addListener: vi.fn(),
		removeListener: vi.fn(),
		dispatchEvent: vi.fn(),
	};

	vi.stubGlobal(
		"matchMedia",
		vi.fn(() => query),
	);
};
afterEach(() => {
	vi.unstubAllGlobals();
});

const ColorSchemeProbe = () => {
	return <span>{usePreferredColorScheme()}</span>;
};

describe("usePreferredColorScheme", () => {
	it("uses the stable default snapshot during server rendering", () => {
		stubMatchMedia(true);

		expect(renderToString(<ColorSchemeProbe />)).toContain(">dark</span>");
	});

	it("reads the browser color scheme after client rendering", () => {
		stubMatchMedia(true);

		const { result } = renderHook(() => usePreferredColorScheme());

		expect(result.current).toBe("light");
	});
});
