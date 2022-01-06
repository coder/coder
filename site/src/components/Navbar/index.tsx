import React from "react"
import { fade, makeStyles } from "@material-ui/core/styles"

export const Navbar: React.FC = () => {
  const styles = useStyles()
  return <div className={styles.root} />
}

const useStyles = makeStyles((theme) => ({
  root: {
    position: "relative",
    height: "56px",
    background: theme.palette.navbar.main,
    marginTop: 0,
    transition: "margin 150ms ease",
    "@media (display-mode: standalone)": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
    borderBottom: `1px solid #383838`,
  },
}))
