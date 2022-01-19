import React from "react"

import { makeStyles } from "@material-ui/core"
import { Navbar } from "../Navbar"
import { Footer } from "./Footer"

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

/**
 * `AppPage` is a main application page - containing the following common elements:
 * - A navbar, with organization dropdown and users
 * - A footer
 */
export const AppPage: React.FC = ({ children }) => {
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
      {children}
      {footer}
    </div>
  )
}
