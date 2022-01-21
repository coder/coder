import React from "react"
import Button from "@material-ui/core/Button"
import List from "@material-ui/core/List"
import ListSubheader from "@material-ui/core/ListSubheader"
import { makeStyles } from "@material-ui/core/styles"
import Link from "next/link"

import { User } from "../../contexts/UserContext"
import { Logo } from "../Icons"

export interface NavbarProps {
  user?: User
}

export const Navbar: React.FC<NavbarProps> = () => {
  const styles = useStyles()
  return (
    <div className={styles.root}>
      <div className={styles.fixed}>
        <Link href="/">
          <Button className={styles.logo} variant="text">
            <Logo fill="white" opacity={1} />
          </Button>
        </Link>
      </div>
      <div className={styles.fullWidth}>
        <div className={styles.title}>Coder v2</div>
      </div>
      <div className={styles.fixed}>
        <List>
          <ListSubheader>Manage</ListSubheader>
        </List>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    position: "relative",
    display: "flex",
    flex: "0",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    height: "56px",
    background: theme.palette.navbar.main,
    marginTop: 0,
    transition: "margin 150ms ease",
    "@media (display-mode: standalone)": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
    borderBottom: `1px solid #383838`,
  },
  fixed: {
    flex: "0",
  },
  fullWidth: {
    flex: "1",
  },
  logo: {
    flex: "0",
    height: "56px",
    paddingLeft: theme.spacing(4),
    paddingRight: theme.spacing(2),
    borderRadius: 0,
    "& svg": {
      display: "block",
      width: 125,
    },
  },
  title: {
    flex: "1",
    textAlign: "center",
  },
}))
