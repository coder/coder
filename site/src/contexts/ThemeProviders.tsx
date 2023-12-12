import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import CssBaseline from "@mui/material/CssBaseline";
import {
  StyledEngineProvider,
  ThemeProvider as MuiThemeProvider,
} from "@mui/material/styles";
import {
  type FC,
  type PropsWithChildren,
  useEffect,
  useMemo,
  useState,
} from "react";
import themes from "theme";
import { useAuth } from "./AuthProvider/AuthProvider";

export const ThemeProviders: FC<PropsWithChildren> = ({ children }) => {
  const { user } = useAuth();
  const themeQuery = useMemo(
    () => window.matchMedia?.("(prefers-color-scheme: light)"),
    [],
  );
  const [preferredColorScheme, setPreferredColorScheme] = useState<
    "dark" | "light"
  >(themeQuery?.matches ? "light" : "dark");

  useEffect(() => {
    const listener = (event: MediaQueryListEvent) => {
      setPreferredColorScheme(event.matches ? "light" : "dark");
    };

    themeQuery.addEventListener("change", listener);
    return () => {
      themeQuery.removeEventListener("change", listener);
    };
  }, [themeQuery]);

  // We might not be logged in yet, or the `theme_preference` could be an empty string.
  const themePreference = user?.theme_preference || "auto";
  // The janky casting here is find because of the much more type safe fallback
  // We need to support `themePreference` being wrong anyway because the database
  // value could be anything, like an empty string.
  const theme =
    themes[themePreference as keyof typeof themes] ??
    themes[preferredColorScheme];

  return (
    <StyledEngineProvider injectFirst>
      <MuiThemeProvider theme={theme}>
        <EmotionThemeProvider theme={theme}>
          <CssBaseline enableColorScheme />
          {children}
        </EmotionThemeProvider>
      </MuiThemeProvider>
    </StyledEngineProvider>
  );
};
