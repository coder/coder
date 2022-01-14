import React from "react"

import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"

import { dark } from "../theme"
import { AppProps } from "next/app"
import { makeStyles } from "@material-ui/core"
import { Navbar } from "../components/Navbar"
import { Footer } from "../components/Page"

/**
 * `Contents` is the wrapper around the core app UI,
 * containing common UI elements like the footer and navbar.
 *
 * This can't be inlined in `MyApp` because it requires styling,
 * and `useStyles` needs to be inside a `<ThemeProvider />`
 */
const Contents: React.FC<AppProps> = ({ Component, pageProps }) => {
  const styles = useStyles()

  const header = (
    <div className={styles.header}>
      <Navbar />
    </div>
  )

  const footer = (
    <div className={styles.footer}>
      <Footer />
    </div>
  )

  return (
    <div className={styles.root}>
      {header}
      <Component {...pageProps} />
      {footer}
    </div>
  )
}

/**
 * <App /> is the root rendering logic of the application - setting up our router
 * and any contexts / global state management.
 */
const MyApp: React.FC<AppProps> = (appProps) => {
  return (
    <ThemeProvider theme={dark}>
      <CssBaseline />
      <Contents {...appProps} />
    </ThemeProvider>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "column",
  },
  header: {
    flex: 0,
  },
  body: {
    height: "100%",
    flex: 1,
  },
  footer: {
    flex: 0,
  },
}))

export default MyApp
