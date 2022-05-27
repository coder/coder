import List from "@material-ui/core/List"
import ListItem from "@material-ui/core/ListItem"
import { fade, makeStyles } from "@material-ui/core/styles"
import React from "react"
import { NavLink } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { navHeight } from "../../theme/constants"
import { Logo } from "../Icons/Logo"
import { UserDropdown } from "../UserDropdown/UsersDropdown"

export interface NavbarViewProps {
  user?: TypesGen.User
  onSignOut: () => void
}

export const Language = {
  workspaces: "Workspaces",
  templates: "Templates",
  users: "Users",
}

export const NavbarView: React.FC<NavbarViewProps> = ({ user, onSignOut }) => {
  const styles = useStyles()
  return (
    <nav className={styles.root}>
      <List className={styles.fixed}>
        <ListItem className={styles.item}>
          <NavLink className={styles.logo} to="/workspaces">
            <Logo fill="white" opacity={1} width={125} />
          </NavLink>
        </ListItem>
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/workspaces">
            {Language.workspaces}
          </NavLink>
        </ListItem>
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/templates">
            {Language.templates}
          </NavLink>
        </ListItem>
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/users">
            {Language.users}
          </NavLink>
        </ListItem>
      </List>
      <div className={styles.fullWidth} />
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
    background: theme.palette.background.paper,
    marginTop: 0,
    transition: "margin 150ms ease",
    "@media (display-mode: standalone)": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
    borderBottom: `1px solid ${theme.palette.divider}`,
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
    paddingRight: theme.spacing(4),
    "& svg": {
      width: 109,
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
      fontWeight: "bold",

      "&::before": {
        content: `" "`,
        bottom: 0,
        left: theme.spacing(3),
        background: "#C16800",
        right: theme.spacing(3),
        height: 2,
        position: "absolute",
      },
    },
  },
}))
