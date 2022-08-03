import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { FC } from "react"
import { HelmetProvider } from "react-helmet-async"
import { BrowserRouter as Router } from "react-router-dom"
import { SWRConfig } from "swr"
import { AppRouter } from "./AppRouter"
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary"
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar"
import { dark } from "./theme"
import "./theme/globalFonts"
import { XServiceProvider } from "./xServices/StateContext"

export const App: FC = () => {
  return (
    <Router>
      <SWRConfig
        value={{
          // This code came from the SWR documentation:
          // https://swr.vercel.app/docs/error-handling#status-code-and-error-object
          fetcher: async (url: string) => {
            const res = await fetch(url)

            // By default, `fetch` won't treat 4xx or 5xx response as errors.
            // However, we want SWR to treat these as errors - so if `res.ok` is false,
            // we want to throw an error to bubble that up to SWR.
            if (!res.ok) {
              const err = new Error((await res.json()).error?.message || res.statusText)
              throw err
            }
            return res.json()
          },
        }}
      >
        <HelmetProvider>
          <ThemeProvider theme={dark}>
            <CssBaseline />
            <ErrorBoundary>
              <XServiceProvider>
                <AppRouter />
                <GlobalSnackbar />
              </XServiceProvider>
            </ErrorBoundary>
          </ThemeProvider>
        </HelmetProvider>
      </SWRConfig>
    </Router>
  )
}
