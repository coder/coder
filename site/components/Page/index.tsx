import React from "react"
import { makeStyles } from "@material-ui/core/styles"

import { Footer } from "./Footer"
import { Navbar } from "../Navbar"

export const Page: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  // TODO: More interesting styling here!

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

  const body = <div className={styles.body}> {children}</div>

  return (
    <div className={styles.root}>
      {header}
      {body}
      {footer}
    </div>
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
