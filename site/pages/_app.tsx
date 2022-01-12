import React from "react"

import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"

import { dark } from "../theme"
import { AppProps } from "next/app"

/**
 * <App /> is the root rendering logic of the application - setting up our router
 * and any contexts / global state management.
 * @returns
 */

const MyApp: React.FC<AppProps> = ({ Component, pageProps }) => {
  return (
    <ThemeProvider theme={dark}>
      <CssBaseline />
      <Component {...pageProps} />
    </ThemeProvider>
  )
}

export default MyApp
