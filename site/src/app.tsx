import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { FC } from "react"
import { BrowserRouter as Router } from "react-router-dom"
import { AppRouter } from "./AppRouter"
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary"
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar"
import { dark } from "./theme"
import "./theme/globalFonts"
import { XServiceProvider } from "./xServices/StateContext"

export const App: FC = () => {
  return (
    <Router>
      <ThemeProvider theme={dark}>
        <CssBaseline />
        <ErrorBoundary>
          <XServiceProvider>
            <AppRouter />
            <GlobalSnackbar />
          </XServiceProvider>
        </ErrorBoundary>
      </ThemeProvider>
    </Router>
  )
}
