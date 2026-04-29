import createCache from "@emotion/cache";
/** @deprecated Emotion is deprecated. Migrate to Tailwind CSS. */
import {
	CacheProvider,
	ThemeProvider as EmotionThemeProvider,
} from "@emotion/react";
/** @deprecated MUI CssBaseline is deprecated. Migrate to shadcn/ui components and Tailwind CSS. */
import CssBaseline from "@mui/material/CssBaseline";
/** @deprecated MUI components are deprecated. Migrate to shadcn/ui components and Tailwind CSS. */
import {
	ThemeProvider as MuiThemeProvider,
	StyledEngineProvider,
} from "@mui/material/styles";
import {
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useEffect,
} from "react";
import { useQuery } from "react-query";
import { appearanceSettings } from "#/api/queries/users";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import themes, { type Theme } from "#/theme";
import { baseModeFor, CONCRETE_THEMES } from "#/theme/colorblind";
import {
	migrateLegacyPreference,
	resolveActiveThemeName,
} from "#/theme/themeMode";
import { usePreferredColorScheme } from "#/theme/usePreferredColorScheme";

/**
 * Root theme provider for the web UI.
 *
 * Decodes the stored appearance settings, including the legacy `auto`
 * value and colorblind-friendly variants, into a concrete theme via
 * `migrateLegacyPreference` and `resolveActiveThemeName`, then:
 *
 * - Applies the concrete theme class to `<html>` (e.g. `dark-tritan`)
 *   plus its base mode class (`dark` or `light`) so Tailwind `dark:`
 *   utilities and any selector-based theming (`.dark` in `Chart.tsx`)
 *   continue to match when a colorblind variant is active.
 * - Watches `prefers-color-scheme` so `auto` preferences follow the
 *   OS.
 * - Skips class manipulation when an embed page has claimed
 *   `<html>` via `data-embed-theme`.
 * - Selects the matching MUI/Emotion theme object and delegates to
 *   the MUI/Emotion providers.
 */
export const ThemeProvider: FC<PropsWithChildren> = ({ children }) => {
	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const preferredColorScheme = usePreferredColorScheme();

	// Prefer the JS-fetched settings; fall back to the SSR meta tag so
	// the first paint picks the right theme even before the React Query
	// response arrives. migrateLegacyPreference tolerates any mix of
	// new/legacy/missing fields.
	const settings =
		appearanceSettingsQuery.data ?? metadata.userAppearance?.value ?? {};
	const state = migrateLegacyPreference(settings);
	const concreteName = resolveActiveThemeName(state, preferredColorScheme);

	useEffect(() => {
		const root = document.documentElement;
		// Embedded pages manage theme independently.
		if (root.dataset.embedTheme) {
			return;
		}
		root.classList.add(concreteName);
		root.classList.add(baseModeFor(concreteName));

		return () => {
			if (!root.dataset.embedTheme) {
				root.classList.remove(...CONCRETE_THEMES);
			}
		};
	}, [concreteName]);

	const theme = themes[concreteName];

	return (
		<StyledEngineProvider injectFirst>
			<ThemeOverride theme={theme}>{children}</ThemeOverride>
		</StyledEngineProvider>
	);
};

// This is being added to allow Tailwind classes to be used with MUI components. https://mui.com/material-ui/integrations/interoperability/#tailwind-css
const cache = createCache({
	key: "css",
	prepend: true,
});

interface ThemeOverrideProps {
	theme: Theme;
	children?: ReactNode;
}

export const ThemeOverride: FC<ThemeOverrideProps> = ({ theme, children }) => {
	return (
		<CacheProvider value={cache}>
			<MuiThemeProvider theme={theme}>
				<EmotionThemeProvider theme={theme}>
					<CssBaseline enableColorScheme />
					{children}
				</EmotionThemeProvider>
			</MuiThemeProvider>
		</CacheProvider>
	);
};
