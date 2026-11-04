import { act, render, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { MockUserAppearanceSettings } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { ThemeProvider } from "./ThemeProvider";

// Differs from the real palette, so a match proves the color was read from
// the stylesheet for the applied theme class.
const lightSurface = "rgb(250, 250, 250)";
const darkSurface = "rgb(20, 20, 30)";

const stubOSColorScheme = (initial: "light" | "dark") => {
	let scheme = initial;
	const listeners = new Set<EventListener>();
	vi.stubGlobal(
		"matchMedia",
		(query: string) =>
			({
				get matches() {
					return query === `(prefers-color-scheme: ${scheme})`;
				},
				media: query,
				onchange: null,
				addEventListener: (_type: string, listener: EventListener) =>
					listeners.add(listener),
				removeEventListener: (_type: string, listener: EventListener) =>
					listeners.delete(listener),
				addListener: () => {},
				removeListener: () => {},
				dispatchEvent: () => true,
			}) satisfies MediaQueryList,
	);
	return (next: typeof scheme) => {
		scheme = next;
		const event = new Event("change");
		for (const listener of listeners) {
			listener(event);
		}
	};
};

const renderThemeProvider = () => {
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<ThemeProvider />
		</QueryClientProvider>,
	);
};

describe("ThemeProvider theme-color", () => {
	let themeColorMetas: HTMLMetaElement[];
	const themeColors = () => themeColorMetas.map((meta) => meta.content);

	beforeEach(() => {
		const style = document.createElement("style");
		style.textContent = `
			.light body { background-color: ${lightSurface}; }
			.dark body { background-color: ${darkSurface}; }
		`;
		themeColorMetas = ["light", "dark"].map((scheme) => {
			const meta = document.createElement("meta");
			meta.name = "theme-color";
			meta.media = `(prefers-color-scheme: ${scheme})`;
			return meta;
		});
		document.head.append(style, ...themeColorMetas);
		return () => {
			style.remove();
			for (const meta of themeColorMetas) {
				meta.remove();
			}
			vi.unstubAllGlobals();
		};
	});

	it("follows the OS color scheme in sync mode", async () => {
		const setOSColorScheme = stubOSColorScheme("light");
		server.use(
			http.get("/api/v2/users/me/appearance", () =>
				HttpResponse.json({
					...MockUserAppearanceSettings,
					theme_mode: "sync",
					theme_light: "light",
					theme_dark: "dark",
				}),
			),
		);

		renderThemeProvider();
		await waitFor(() =>
			expect(themeColors()).toEqual([lightSurface, lightSurface]),
		);

		act(() => setOSColorScheme("dark"));
		expect(themeColors()).toEqual([darkSurface, darkSurface]);
	});

	it("uses a pinned theme over the OS color scheme", async () => {
		stubOSColorScheme("dark");
		server.use(
			http.get("/api/v2/users/me/appearance", () =>
				HttpResponse.json({
					...MockUserAppearanceSettings,
					theme_mode: "single",
					theme_preference: "light",
				}),
			),
		);

		renderThemeProvider();
		await waitFor(() =>
			expect(themeColors()).toEqual([lightSurface, lightSurface]),
		);
	});
});
