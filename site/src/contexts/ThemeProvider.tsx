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
import themes, { baseModeFor, CONCRETE_THEMES, type Theme } from "#/theme";
import {
	migrateLegacyPreference,
	resolveActiveThemeName,
} from "#/theme/themeMode";
import { usePreferredColorScheme } from "#/theme/usePreferredColorScheme";

export const ThemeProvider: FC<PropsWithChildren> = ({ children }) => {
	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const preferredColorScheme = usePreferredColorScheme();

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
