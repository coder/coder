import List from "@material-ui/core/List"
import ListItem from "@material-ui/core/ListItem"
import { fade, makeStyles } from "@material-ui/core/styles"
import React from "react"
import { NavLink } from "react-router-dom"
import { UserResponse } from "../../../api/types"
import { navHeight } from "../../../theme/constants"
import { AdminDropdown } from "../../AdminDropdown/AdminDropdown"
import { Logo } from "../../Icons/Logo"
import { UserDropdown } from "../UserDropdown"

export interface NavbarViewProps {
  user?: UserResponse
  onSignOut: () => void
}

export const NavbarView: React.FC<NavbarViewProps> = ({ user, onSignOut }) => {
  const styles = useStyles()
  return (
    <nav className={styles.root}>
      <List className={styles.fixed}>
        <ListItem className={styles.item}>
          <NavLink className={styles.logo} to="/">
            <Logo fill="white" opacity={1} width={125} />
          </NavLink>
        </ListItem>
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/templates">
            Templates
          </NavLink>
        </ListItem>
      </List>
      <div className={styles.fullWidth} />
      {user && user.email === "admin@coder.com" && <AdminDropdown />}
      <div className={styles.fixed}>{user && <UserDropdown user={user} onSignOut={onSignOut} />}</div>
    </nav>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    position: "relative",
    display: "flex",
    flex: 0,
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    height: navHeight,
    background: theme.palette.navbar.main,
    marginTop: 0,
    transition: "margin 150ms ease",
    "@media (display-mode: standalone)": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
    borderBottom: `1px solid #383838`,
  },
  fixed: {
    flex: 0,
    display: "flex",
    padding: 0,
  },
  fullWidth: {
    flex: 1,
  },
  logo: {
    alignItems: "center",
    display: "flex",
    height: navHeight,
    paddingLeft: theme.spacing(4),
    paddingRight: theme.spacing(2),
    "& svg": {
      width: 125,
    },
  },
  title: {
    flex: 1,
    textAlign: "center",
  },
  item: {
    padding: 0,
  },
  link: {
    alignItems: "center",
    color: "#A7A7A7",
    display: "flex",
    fontSize: 16,
    height: navHeight,
    padding: `0 ${theme.spacing(3)}px`,
    textDecoration: "none",
    transition: "background-color 0.3s ease",

    "&:hover": {
      backgroundColor: fade(theme.palette.primary.light, 0.1),
    },

    // NavLink adds this class when the current route matches.
    "&.active": {
      position: "relative",
      color: theme.palette.primary.contrastText,

      "&::before": {
        content: `"{"`,
        left: 10,
        position: "absolute",
      },

      "&::after": {
        content: `"}"`,
        position: "absolute",
        right: 10,
      },
    },
  },
}))
