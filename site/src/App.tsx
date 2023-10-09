import CssBaseline from "@mui/material/CssBaseline";
import { QueryClient, QueryClientProvider } from "react-query";
import { AuthProvider } from "components/AuthProvider/AuthProvider";
import { FC, PropsWithChildren } from "react";
import { HelmetProvider } from "react-helmet-async";
import { AppRouter } from "./AppRouter";
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary";
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar";
import { dark } from "./theme";
import "./theme/globalFonts";
import {
  StyledEngineProvider,
  ThemeProvider as MuiThemeProvider,
} from "@mui/material/styles";
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      cacheTime: 0,
      refetchOnWindowFocus: false,
      networkMode: "offlineFirst",
    },
  },
});

export const AppProviders: FC<PropsWithChildren> = ({ children }) => {
  return (
    <HelmetProvider>
      <StyledEngineProvider injectFirst>
        <MuiThemeProvider theme={dark}>
          <EmotionThemeProvider theme={dark}>
            <CssBaseline enableColorScheme />
            <ErrorBoundary>
              <QueryClientProvider client={queryClient}>
                <AuthProvider>
                  {children}
                  <GlobalSnackbar />
                </AuthProvider>
              </QueryClientProvider>
            </ErrorBoundary>
          </EmotionThemeProvider>
        </MuiThemeProvider>
      </StyledEngineProvider>
    </HelmetProvider>
  );
};

export const App: FC = () => {
  return (
    <AppProviders>
      <AppRouter />
    </AppProviders>
  );
};
