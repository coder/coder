import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { AuthProvider } from "components/AuthProvider/AuthProvider"
import { FC } from "react"
import { HelmetProvider } from "react-helmet-async"
import { AppRouter } from "./AppRouter"
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary"
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar"
import { dark } from "./theme"
import "./theme/globalFonts"

export const App: FC = () => {
  return (
    <HelmetProvider>
      <ThemeProvider theme={dark}>
        <CssBaseline />
        <ErrorBoundary>
          <AuthProvider>
            <AppRouter />
            <GlobalSnackbar />
          </AuthProvider>
        </ErrorBoundary>
      </ThemeProvider>
    </HelmetProvider>
  )
}
