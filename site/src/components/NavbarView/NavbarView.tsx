import Drawer from "@material-ui/core/Drawer"
import IconButton from "@material-ui/core/IconButton"
import List from "@material-ui/core/List"
import ListItem from "@material-ui/core/ListItem"
import { makeStyles } from "@material-ui/core/styles"
import MenuIcon from "@material-ui/icons/Menu"
import { useState } from "react"
import { NavLink, useLocation } from "react-router-dom"
import { colors } from "theme/colors"
import * as TypesGen from "../../api/typesGenerated"
import { containerWidth, navHeight, sidePadding } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { Logo } from "../Icons/Logo"
import { UserDropdown } from "../UserDropdown/UsersDropdown"

export interface NavbarViewProps {
  user?: TypesGen.User
  onSignOut: () => void
  canViewAuditLog: boolean
  canViewDeployment: boolean
}

export const Language = {
  workspaces: "Workspaces",
  templates: "Templates",
  users: "Users",
  audit: "Audit",
  deployment: "Deployment",
}

const NavItems: React.FC<
  React.PropsWithChildren<{
    className?: string
    canViewAuditLog: boolean
    canViewDeployment: boolean
  }>
> = ({ className, canViewAuditLog, canViewDeployment }) => {
  const styles = useStyles()
  const location = useLocation()

  return (
    <List className={combineClasses([styles.navItems, className])}>
      <ListItem button className={styles.item}>
        <NavLink
          className={combineClasses([
            styles.link,
            location.pathname.startsWith("/@") && "active",
          ])}
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
        <NavLink
          className={styles.link}
          to={`/users?filter=${encodeURIComponent("status:active")}`}
        >
          {Language.users}
        </NavLink>
      </ListItem>
      {canViewAuditLog && (
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/audit">
            {Language.audit}
          </NavLink>
        </ListItem>
      )}
      {canViewDeployment && (
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/settings/deployment/general">
            {Language.deployment}
          </NavLink>
        </ListItem>
      )}
    </List>
  )
}
export const NavbarView: React.FC<React.PropsWithChildren<NavbarViewProps>> = ({
  user,
  onSignOut,
  canViewAuditLog,
  canViewDeployment,
}) => {
  const styles = useStyles()
  const [isDrawerOpen, setIsDrawerOpen] = useState(false)

  return (
    <nav className={styles.root}>
      <div className={styles.wrapper}>
        <IconButton
          aria-label="Open menu"
          className={styles.mobileMenuButton}
          onClick={() => {
            setIsDrawerOpen(true)
          }}
        >
          <MenuIcon />
        </IconButton>

        <Drawer
          anchor="left"
          open={isDrawerOpen}
          onClose={() => setIsDrawerOpen(false)}
        >
          <div className={styles.drawer}>
            <div className={styles.drawerHeader}>
              <Logo fill="white" opacity={1} width={125} />
            </div>
            <NavItems
              canViewAuditLog={canViewAuditLog}
              canViewDeployment={canViewDeployment}
            />
          </div>
        </Drawer>

        <NavLink className={styles.logo} to="/workspaces">
          <Logo fill="white" opacity={1} width={125} />
        </NavLink>

        <NavItems
          className={styles.desktopNavItems}
          canViewAuditLog={canViewAuditLog}
          canViewDeployment={canViewDeployment}
        />

        <div className={styles.profileButton}>
          {user && <UserDropdown user={user} onSignOut={onSignOut} />}
        </div>
      </div>
    </nav>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    height: navHeight,
    background: theme.palette.background.paper,
    "@media (display-mode: standalone)": {
      borderTop: `1px solid ${theme.palette.divider}`,
    },
    borderBottom: `1px solid ${theme.palette.divider}`,
    transition: "margin 150ms ease",
  },
  wrapper: {
    position: "relative",
    display: "flex",
    justifyContent: "space-between",
    alignItems: "center",
    margin: "0 auto",
    maxWidth: containerWidth,
    padding: `0 ${sidePadding}px`,
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
    color: colors.gray[6],
    display: "flex",
    flex: 1,
    fontSize: 16,
    padding: `${theme.spacing(1.5)}px ${theme.spacing(2)}px`,
    textDecoration: "none",
    transition: "background-color 0.15s ease-in-out",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },

    // NavLink adds this class when the current route matches.
    "&.active": {
      position: "relative",
      color: theme.palette.text.primary,
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
