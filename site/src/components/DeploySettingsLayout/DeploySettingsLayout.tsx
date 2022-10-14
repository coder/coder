import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import LaunchOutlined from "@material-ui/icons/LaunchOutlined"
import VpnKeyOutlined from "@material-ui/icons/VpnKeyOutlined"
import LockRounded from "@material-ui/icons/LockRounded"
import BarChartOutlined from "@material-ui/icons/BarChartOutlined"
import { Margins } from "components/Margins/Margins"
import { Stack } from "components/Stack/Stack"
import React, { ElementType, PropsWithChildren, ReactNode } from "react"
import { NavLink } from "react-router-dom"
import { combineClasses } from "util/combineClasses"

const Sidebar: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <nav className={styles.sidebar}>{children}</nav>
}

const SidebarNavItem: React.FC<
  PropsWithChildren<{ href: string; icon: ReactNode }>
> = ({ children, href, icon }) => {
  const styles = useStyles()
  return (
    <NavLink
      to={href}
      className={({ isActive }) =>
        combineClasses([
          styles.sidebarNavItem,
          isActive ? styles.sidebarNavItemActive : undefined,
        ])
      }
    >
      <Stack alignItems="center" spacing={1.5} direction="row">
        {icon}
        {children}
      </Stack>
    </NavLink>
  )
}

const SidebarNavItemIcon: React.FC<{ icon: ElementType }> = ({
  icon: Icon,
}) => {
  const styles = useStyles()
  return <Icon className={styles.sidebarNavItemIcon} />
}

export const SettingsHeader: React.FC<{
  title: string
  description: string | JSX.Element
  docsHref: string
}> = ({ title, description, docsHref }) => {
  const styles = useStyles()

  return (
    <Stack alignItems="baseline" direction="row" justifyContent="space-between">
      <div className={styles.headingGroup}>
        <h1 className={styles.title}>{title}</h1>
        <span className={styles.description}>{description}</span>
      </div>

      <Button
        size="small"
        startIcon={<LaunchOutlined />}
        component="a"
        href={docsHref}
        target="_blank"
        variant="outlined"
      >
        Read the docs
      </Button>
    </Stack>
  )
}

export const SettingsBadges: React.FC<{
  isEnterprise?: boolean
  isEnabled?: boolean
}> = ({ isEnterprise, isEnabled }) => {
  const styles = useStyles()

  return (
    <Stack
      direction="row"
      alignItems="center"
      className={styles.badges}
      spacing={1}
    >
      {isEnabled ? (
        <span className={styles.enabledBadge}>Enabled</span>
      ) : (
        <span className={styles.disabledBadge}>Enabled</span>
      )}
      {isEnterprise ? (
        <span className={styles.enterpriseBadge}>Enterprise</span>
      ) : null}
    </Stack>
  )
}

export const DeploySettingsLayout: React.FC<PropsWithChildren> = ({
  children,
}) => {
  const styles = useStyles()

  return (
    <Margins>
      <Stack className={styles.wrapper} direction="row" spacing={5}>
        <Sidebar>
          <SidebarNavItem
            href="/settings/general"
            icon={<SidebarNavItemIcon icon={LaunchOutlined} />}
          >
            General
          </SidebarNavItem>
          <SidebarNavItem
            href="/settings/security"
            icon={<SidebarNavItemIcon icon={LockRounded} />}
          >
            Security
          </SidebarNavItem>
          <SidebarNavItem
            href="/settings/metrics"
            icon={<SidebarNavItemIcon icon={BarChartOutlined} />}
          >
            Metrics / observability
          </SidebarNavItem>
          <SidebarNavItem
            href="/settings/auth"
            icon={<SidebarNavItemIcon icon={VpnKeyOutlined} />}
          >
            Authentication
          </SidebarNavItem>
        </Sidebar>

        <main className={styles.content}>{children}</main>
      </Stack>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(6, 0),
  },

  sidebar: {
    width: 245,
  },

  sidebarNavItem: {
    color: "inherit",
    display: "block",
    fontSize: 16,
    textDecoration: "none",
    padding: theme.spacing(1.5, 1.5, 1.5, 3),
    borderRadius: theme.shape.borderRadius / 2,
    transition: "background-color 0.15s ease-in-out",
    marginBottom: 1,
    position: "relative",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  },

  sidebarNavItemActive: {
    backgroundColor: theme.palette.action.hover,

    "&:before": {
      content: '""',
      display: "block",
      width: 3,
      height: "100%",
      position: "absolute",
      left: 0,
      top: 0,
      backgroundColor: theme.palette.secondary.dark,
      borderRadius: theme.shape.borderRadius,
    },
  },

  sidebarNavItemIcon: {
    width: theme.spacing(2),
    height: theme.spacing(2),
  },

  content: {
    maxWidth: 800,
    width: "100%",
  },

  headingGroup: {
    maxWidth: 420,
    marginBottom: theme.spacing(4),
  },

  title: {
    fontSize: 36,
    fontWeight: 700,
    display: "flex",
    alignItems: "center",
    lineHeight: "initial",
    margin: 0,
    marginBottom: theme.spacing(0.5),
  },

  description: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
  },

  badges: {
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
  },

  enterpriseBadge: {
    fontSize: 10,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: "0.085em",
    backgroundColor: theme.palette.info.dark,
    padding: theme.spacing(0.5, 2),
    borderRadius: 9999,
    border: `1px solid ${theme.palette.info.light}`,
    lineHeight: "160%",
  },

  enabledBadge: {
    fontSize: 10,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: "0.085em",
    backgroundColor: theme.palette.success.dark,
    padding: theme.spacing(0.5, 2),
    borderRadius: 9999,
    border: `1px solid ${theme.palette.success.light}`,
    lineHeight: "160%",
  },

  disabledBadge: {
    fontSize: 10,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: "0.085em",
    backgroundColor: theme.palette.background.paper,
    padding: theme.spacing(0.5, 2),
    borderRadius: 9999,
    border: `1px solid ${theme.palette.divider}`,
    lineHeight: "160%",
  },
}))
