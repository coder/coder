import React from "react"
import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { SWRConfig } from "swr"
import { AppProps } from "next/app"
import { UserProvider } from "../contexts/UserContext"
import { light } from "../theme"

/**
 * ClientRender is a component that only allows its children to be rendered
 * client-side. This check is performed by querying the existence of the window
 * global.
 */
const ClientRender: React.FC = ({ children }) => (
  <div suppressHydrationWarning>{typeof window === "undefined" ? null : children}</div>
)

/**
 * <App /> is the root rendering logic of the application - setting up our router
 * and any contexts / global state management.
 */
const MyApp: React.FC<AppProps> = ({ Component, pageProps }) => {
  return (
    <ClientRender>
      <SWRConfig
        value={{
          fetcher: async (url: string) => {
            const res = await fetch(url)
            if (!res.ok) {
              const err = new Error((await res.json()).error?.message || res.statusText)
              throw err
            }
            return res.json()
          },
        }}
      >
        <UserProvider>
          <ThemeProvider theme={light}>
            <CssBaseline />
            <Component {...pageProps} />
          </ThemeProvider>
        </UserProvider>
      </SWRConfig>
    </ClientRender>
  )
}

export default MyApp
