import React from "react"
import { fade, makeStyles } from "@material-ui/core/styles"
import Button from "@material-ui/core/Button"
import { Link, useLocation } from "react-router-dom"
import { Logo } from "./../Icons/Logo"

export const Navbar: React.FC = () => {
  const styles = useStyles()
  return <div className={styles.root}>
    <Link to="/">
      <Button className={styles.logo} variant="text">
        <Logo fill="white" opacity={1} />
      </Button>
    </Link>
  </div>
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
  logo: {
    height: "56px",
    paddingLeft: theme.spacing(4),
    paddingRight: theme.spacing(2),
    margin: "0 auto",
    borderRadius: 0,
    "& svg": {
      display: "block",
      width: 125,
    },
  },
}))
