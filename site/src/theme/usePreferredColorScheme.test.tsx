import { act, renderHook } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { usePreferredColorScheme } from "./usePreferredColorScheme";

const stubMatchMedia = (matches: boolean) => {
	const query = {
		matches,
		media: "(prefers-color-scheme: light)",
		onchange: null,
		addEventListener: vi.fn<MediaQueryList["addEventListener"]>(),
		removeEventListener: vi.fn<MediaQueryList["removeEventListener"]>(),
		addListener: vi.fn<MediaQueryList["addListener"]>(),
		removeListener: vi.fn<MediaQueryList["removeListener"]>(),
		dispatchEvent: vi.fn<MediaQueryList["dispatchEvent"]>(() => true),
	} satisfies MediaQueryList;

	vi.stubGlobal(
		"matchMedia",
		vi.fn(() => query),
	);

	return query;
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

	it("updates when the browser color scheme changes", () => {
		const query = stubMatchMedia(true);

		const { result } = renderHook(() => usePreferredColorScheme());

		expect(result.current).toBe("light");

		const changeListener = query.addEventListener.mock.calls.find(
			([eventName]) => eventName === "change",
		)?.[1];
		if (typeof changeListener !== "function") {
			throw new Error("Expected change listener to be registered.");
		}

		query.matches = false;
		act(() => {
			changeListener(new Event("change"));
		});

		expect(result.current).toBe("dark");
	});
});
