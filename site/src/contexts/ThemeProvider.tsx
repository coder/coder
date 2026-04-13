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
	useMemo,
	useState,
} from "react";
import { useQuery } from "react-query";
import { appearanceSettings } from "#/api/queries/users";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { type Theme, type ThemeName, themes } from "#/theme";

/**
 * We use theme names as classnames on the html root. Get the string values
 * here.
 */
const THEME_NAMES = Object.keys(themes);

const isValidThemeName = (name: string | undefined): name is ThemeName =>
	name !== undefined && name in themes;

/**
 * Returns the user's selected theme in their settings. If the user has "auto"
 * selected, or we cannot determine the theme for whatever reason, returns
 * undefined
 */
const useUserAppearanceTheme = (): ThemeName | undefined => {
	const { metadata } = useEmbeddedMetadata();
	const { data } = useQuery(appearanceSettings(metadata.userAppearance));

	const themePreference = data?.theme_preference;

	// if the user's theme preference is "auto", bail so we can fall back
	// to system pref
	if (themePreference === "auto") {
		return;
	}

	if (isValidThemeName(themePreference)) {
		return themePreference;
	}
};

/**
 * Returns the theme name that corresponds to the user's system preference.
 */
const useSystemPreferenceTheme = (): ThemeName => {
	const themeQuery = useMemo(
		() => window.matchMedia?.("(prefers-color-scheme: light)"),
		[],
	);
	const [preferredColorScheme, setPreferredColorScheme] = useState<ThemeName>(
		themeQuery?.matches ? "light" : "dark",
	);

	useEffect(() => {
		if (!themeQuery) {
			return;
		}

		const listener = (event: MediaQueryListEvent) => {
			setPreferredColorScheme(event.matches ? "light" : "dark");
		};

		// `addEventListener` here is a recent API that only _very_ up-to-date
		// browsers support, and that isn't mocked in Jest.
		themeQuery.addEventListener?.("change", listener);
		return () => {
			themeQuery.removeEventListener?.("change", listener);
		};
	}, [themeQuery]);

	return preferredColorScheme;
};

/**
 * Determines the application theme. Tries to get the theme name from the meta
 * tags, the user's appearance settings (as returned by the API) and their
 * system appearance pref as a fallback
 */
export const ThemeProvider: FC<PropsWithChildren> = ({ children }) => {
	const userAppearanceTheme = useUserAppearanceTheme();
	const systemTheme = useSystemPreferenceTheme();

	const themePreference = userAppearanceTheme || systemTheme;

	useEffect(() => {
		const root = document.documentElement;
		// Embedded pages manage theme independently.
		if (root.dataset.embedTheme) {
			return;
		}

		root.classList.remove(...THEME_NAMES);
		root.classList.add(themePreference);
	}, [themePreference]);

	const theme = themes[themePreference];

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
