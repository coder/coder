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
import themes, {
	baseModeFor,
	CONCRETE_THEMES,
	DEFAULT_THEME,
	resolveThemeName,
	type Theme,
} from "#/theme";

export const ThemeProvider: FC<PropsWithChildren> = ({ children }) => {
	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const themeQuery = useMemo(
		() => window.matchMedia?.("(prefers-color-scheme: light)"),
		[],
	);
	const [preferredColorScheme, setPreferredColorScheme] = useState<
		"dark" | "light"
	>(themeQuery?.matches ? "light" : "dark");

	useEffect(() => {
		if (!themeQuery) {
			return;
		}

		const listener = (event: MediaQueryListEvent) => {
			setPreferredColorScheme(event.matches ? "light" : "dark");
		};

		// `addEventListener` here is a recent API that isn't mocked in tests.
		themeQuery.addEventListener?.("change", listener);
		return () => {
			themeQuery.removeEventListener?.("change", listener);
		};
	}, [themeQuery]);

	// We might not be logged in yet, or the `theme_preference` could be an
	// empty string. Prefer the JS-fetched value, fall back to the
	// server-rendered meta tag, then to DEFAULT_THEME.
	const storedPreference =
		appearanceSettingsQuery.data?.theme_preference ||
		metadata.userAppearance?.value?.theme_preference ||
		DEFAULT_THEME;
	const concreteName = resolveThemeName(storedPreference, preferredColorScheme);

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
