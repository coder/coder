import createCache from "@emotion/cache";
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import { CacheProvider } from "@emotion/react";
import CssBaseline from "@mui/material/CssBaseline";
import {
	ThemeProvider as MuiThemeProvider,
	StyledEngineProvider,
	// biome-ignore lint/nursery/noRestrictedImports: we extend the MUI theme
} from "@mui/material/styles";
import {
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useContext,
	useEffect,
	useMemo,
	useState,
} from "react";
import themes, { DEFAULT_THEME, type Theme } from "theme";
import { AuthContext } from "./auth/AuthProvider";

/**
 *
 */
export const ThemeProvider: FC<PropsWithChildren> = ({ children }) => {
	// We need to use the `AuthContext` directly, rather than the `useAuth` hook,
	// because Storybook and many tests depend on this component, but do not provide
	// an `AuthProvider`, and `useAuth` will throw in that case.
	const user = useContext(AuthContext)?.user;
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

		// `addEventListener` here is a recent API that only _very_ up-to-date
		// browsers support, and that isn't mocked in Jest.
		themeQuery.addEventListener?.("change", listener);
		return () => {
			themeQuery.removeEventListener?.("change", listener);
		};
	}, [themeQuery]);

	// We might not be logged in yet, or the `theme_preference` could be an empty string.
	const themePreference = user?.theme_preference || DEFAULT_THEME;
	// The janky casting here is find because of the much more type safe fallback
	// We need to support `themePreference` being wrong anyway because the database
	// value could be anything, like an empty string.

	useEffect(() => {
		const root = document.documentElement;
		if (themePreference === "auto") {
			root.classList.add(preferredColorScheme);
		} else {
			root.classList.add(themePreference);
		}

		return () => {
			root.classList.remove("light", "dark");
		};
	}, [themePreference, preferredColorScheme]);

	const theme =
		themes[themePreference as keyof typeof themes] ??
		themes[preferredColorScheme];

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
