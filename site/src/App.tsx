import CssBaseline from "@mui/material/CssBaseline";
import { QueryClient, QueryClientProvider } from "react-query";
import { AuthProvider } from "components/AuthProvider/AuthProvider";
import type { FC, PropsWithChildren, ReactNode } from "react";
import { HelmetProvider } from "react-helmet-async";
import { AppRouter } from "./AppRouter";
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary";
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar";
import { dark } from "./theme/mui";
import { dark as experimental } from "./theme/experimental";
import "./theme/globalFonts";
import {
  StyledEngineProvider,
  ThemeProvider as MuiThemeProvider,
} from "@mui/material/styles";
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";

const shouldEnableCache =
  window.location.hostname.includes("dev.coder.com") ||
  process.env.NODE_ENV === "development";

const defaultQueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      refetchOnWindowFocus: false,
      cacheTime: shouldEnableCache ? undefined : 0,
      networkMode: shouldEnableCache ? undefined : "offlineFirst",
    },
  },
});

const theme = {
  ...dark,
  experimental,
};

export const ThemeProviders: FC<PropsWithChildren> = ({ children }) => {
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

export const AppProviders = ({
  children,
  queryClient = defaultQueryClient,
}: {
  children: ReactNode;
  queryClient?: QueryClient;
}) => {
  return (
    <HelmetProvider>
      <ThemeProviders>
        <ErrorBoundary>
          <QueryClientProvider client={queryClient}>
            <AuthProvider>
              {children}
              <GlobalSnackbar />
            </AuthProvider>
          </QueryClientProvider>
        </ErrorBoundary>
      </ThemeProviders>
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
