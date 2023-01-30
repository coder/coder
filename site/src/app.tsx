import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { AuthProvider } from "components/AuthProvider/AuthProvider"
import { FC, PropsWithChildren } from "react"
import { HelmetProvider } from "react-helmet-async"
import { AppRouter } from "./AppRouter"
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary"
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar"
import { dark } from "./theme"
import "./theme/globalFonts"

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      cacheTime: 0,
    },
  },
})

export const AppProviders: FC<PropsWithChildren> = ({ children }) => {
  return (
    <HelmetProvider>
      <ThemeProvider theme={dark}>
        <CssBaseline />
        <ErrorBoundary>
          <QueryClientProvider client={queryClient}>
            <AuthProvider>
              {children}
              <GlobalSnackbar />
            </AuthProvider>
          </QueryClientProvider>
        </ErrorBoundary>
      </ThemeProvider>
    </HelmetProvider>
  )
}

export const App: FC = () => {
  return (
    <AppProviders>
      <AppRouter />
    </AppProviders>
  )
}
