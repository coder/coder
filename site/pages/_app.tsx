import React from "react"

import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"

import { light } from "../theme"
import { AppProps } from "next/app"
import { makeStyles } from "@material-ui/core"

/**
 * SafeHydrate is a component that only allows its children to be rendered
 * client-side. This check is performed by querying the existence of the window
 * global.
 */
const SafeHydrate: React.FC = ({ children }) => (
  <div suppressHydrationWarning>{typeof window === "undefined" ? null : children}</div>
)

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
      <ThemeProvider theme={light}>
        <CssBaseline />
        <Component {...pageProps} />
      </ThemeProvider>
    </ClientRender>
  )
}

export default MyApp
