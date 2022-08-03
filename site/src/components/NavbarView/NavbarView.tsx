import Drawer from "@material-ui/core/Drawer"
import IconButton from "@material-ui/core/IconButton"
import List from "@material-ui/core/List"
import ListItem from "@material-ui/core/ListItem"
import { fade, makeStyles } from "@material-ui/core/styles"
import MenuIcon from "@material-ui/icons/Menu"
import { useState } from "react"
import { NavLink, useLocation } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { navHeight } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
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

const NavItems: React.FC<
  React.PropsWithChildren<{ className?: string; linkClassName?: string }>
> = ({ className }) => {
  const styles = useStyles()
  const location = useLocation()

  return (
    <List className={combineClasses([styles.navItems, className])}>
      <ListItem button className={styles.item}>
        <NavLink
          className={combineClasses([styles.link, location.pathname.startsWith("/@") && "active"])}
          to="/workspaces"
        >
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
  )
}

export const NavbarView: React.FC<React.PropsWithChildren<NavbarViewProps>> = ({
  user,
  onSignOut,
}) => {
  const styles = useStyles()
  const [isDrawerOpen, setIsDrawerOpen] = useState(false)

  return (
    <nav className={styles.root}>
      <IconButton
        aria-label="Open menu"
        className={styles.mobileMenuButton}
        onClick={() => {
          setIsDrawerOpen(true)
        }}
      >
        <MenuIcon />
      </IconButton>

      <Drawer anchor="left" open={isDrawerOpen} onClose={() => setIsDrawerOpen(false)}>
        <div className={styles.drawer}>
          <div className={styles.drawerHeader}>
            <Logo fill="white" opacity={1} width={125} />
          </div>
          <NavItems />
        </div>
      </Drawer>

      <NavLink className={styles.logo} to="/workspaces">
        <Logo fill="white" opacity={1} width={125} />
      </NavLink>

      <NavItems className={styles.desktopNavItems} />

      <div className={styles.profileButton}>
        {user && <UserDropdown user={user} onSignOut={onSignOut} />}
      </div>
    </nav>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    position: "relative",
    display: "flex",
    justifyContent: "space-between",
    alignItems: "center",
    height: navHeight,
    background: theme.palette.background.paper,
    marginTop: 0,
    transition: "margin 150ms ease",
    "@media (display-mode: standalone)": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
    borderBottom: `1px solid ${theme.palette.divider}`,

    [theme.breakpoints.up("md")]: {
      justifyContent: "flex-start",
    },
  },
  drawer: {
    width: 250,
  },
  drawerHeader: {
    padding: theme.spacing(2),
    paddingTop: theme.spacing(4),
    paddingBottom: theme.spacing(4),
  },
  navItems: {
    padding: 0,
  },
  desktopNavItems: {
    display: "none",
    [theme.breakpoints.up("md")]: {
      display: "flex",
    },
  },
  profileButton: {
    [theme.breakpoints.up("md")]: {
      marginLeft: "auto",
    },
  },
  mobileMenuButton: {
    [theme.breakpoints.up("md")]: {
      display: "none",
    },
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
    color: "hsl(220, 11%, 71%)",
    display: "flex",
    fontSize: 16,
    padding: `${theme.spacing(1.5)}px ${theme.spacing(2)}px`,
    textDecoration: "none",
    transition: "background-color 0.3s ease",

    "&:hover": {
      backgroundColor: fade(theme.palette.primary.light, 0.05),
    },

    // NavLink adds this class when the current route matches.
    "&.active": {
      position: "relative",
      color: theme.palette.primary.contrastText,
      fontWeight: "bold",

      "&::before": {
        content: `" "`,
        left: 0,
        width: 2,
        height: "100%",
        background: theme.palette.secondary.dark,
        position: "absolute",

        [theme.breakpoints.up("md")]: {
          bottom: 0,
          left: theme.spacing(3),
          width: `calc(100% - 2 * ${theme.spacing(3)}px)`,
          right: theme.spacing(3),
          height: 2,
        },
      },
    },

    [theme.breakpoints.up("md")]: {
      height: navHeight,
      padding: `0 ${theme.spacing(3)}px`,
    },
  },
}))
