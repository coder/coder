import CssBaseline from "@mui/material/CssBaseline"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { AuthProvider } from "components/AuthProvider/AuthProvider"
import { FC, PropsWithChildren } from "react"
import { HelmetProvider } from "react-helmet-async"
import { AppRouter, AppRouterProps } from "./AppRouter"
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary"
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar"
import { dark } from "./theme"
import "./theme/globalFonts"
import { StyledEngineProvider, ThemeProvider } from "@mui/material/styles"
import { BrowserRouter } from "react-router-dom"

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      cacheTime: 0,
      refetchOnWindowFocus: false,
      networkMode: "offlineFirst",
    },
  },
})

export const AppProviders: FC<PropsWithChildren> = ({ children }) => {
  return (
    <HelmetProvider>
      <StyledEngineProvider injectFirst>
        <ThemeProvider theme={dark}>
          <CssBaseline enableColorScheme />
          <ErrorBoundary>
            <QueryClientProvider client={queryClient}>
              <AuthProvider>
                {children}
                <GlobalSnackbar />
              </AuthProvider>
            </QueryClientProvider>
          </ErrorBoundary>
        </ThemeProvider>
      </StyledEngineProvider>
    </HelmetProvider>
  )
}

type AppProps = {
  router?: AppRouterProps
}

export const App: FC<AppProps> = ({
  router = { component: BrowserRouter },
}) => {
  return (
    <AppProviders>
      <AppRouter {...router} />
    </AppProviders>
  )
}
