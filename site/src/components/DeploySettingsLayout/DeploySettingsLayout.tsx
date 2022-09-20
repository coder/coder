import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import LaunchOutlined from "@material-ui/icons/LaunchOutlined"
import VpnKeyOutlined from "@material-ui/icons/VpnKeyOutlined"
import { Margins } from "components/Margins/Margins"
import { Stack } from "components/Stack/Stack"
import React, { ElementType, PropsWithChildren, ReactNode } from "react"
import { NavLink } from "react-router-dom"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import { combineClasses } from "util/combineClasses"

const Sidebar: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <nav className={styles.sidebar}>{children}</nav>
}

const SidebarNavItem: React.FC<PropsWithChildren<{ href: string; icon: ReactNode }>> = ({
  children,
  href,
  icon,
}) => {
  const styles = useStyles()
  return (
    <NavLink
      to={href}
      className={({ isActive }) =>
        combineClasses([styles.sidebarNavItem, isActive ? styles.sidebarNavItemActive : undefined])
      }
    >
      <Stack alignItems="center" spacing={1.5} direction="row">
        {icon}
        {children}
      </Stack>
    </NavLink>
  )
}

const SidebarNavItemIcon: React.FC<{ icon: ElementType }> = ({ icon: Icon }) => {
  const styles = useStyles()
  return <Icon className={styles.sidebarNavItemIcon} />
}

const SidebarCaption: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <span className={styles.sidebarCaption}>{children}</span>
}

export const SettingsHeader: React.FC<{
  title: string
  description: string | JSX.Element
  isEnterprise?: boolean
  docsHref: string
}> = ({ title, description, isEnterprise, docsHref }) => {
  const styles = useStyles()

  return (
    <Stack alignItems="baseline" direction="row" justifyContent="space-between">
      <div className={styles.headingGroup}>
        <h1 className={styles.title}>
          {title}
          {isEnterprise ? <span className={styles.enterpriseBadge}>Enterprise</span> : null}
        </h1>
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

export const SettingsList: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()

  return <div className={styles.settingsList}>{children}</div>
}

const SettingsValue: React.FC<{ label: string; value: string; type?: "primary" | "secondary" }> = ({
  label,
  value,
  type = "primary",
}) => {
  const styles = useStyles()

  return (
    <div>
      <span className={styles.settingsValueLabel}>{label}</span>
      <span
        className={combineClasses([
          styles.settingsValueValue,
          type === "secondary" ? styles.settingsValueSecondary : undefined,
        ])}
      >
        {value}
      </span>
    </div>
  )
}

export const SettingsItem: React.FC<{
  title: string
  description: string | JSX.Element
  values: { label: string; value: string }[]
}> = ({ title, description, values }) => {
  const styles = useStyles()

  return (
    <section className={styles.settingsItem}>
      <div>
        <h2 className={styles.settingsItemTitle}>{title}</h2>
        <span className={styles.settingsItemDescription}>{description}</span>
      </div>

      <Stack alignItems="baseline" direction="row" className={styles.settingsValues} spacing={5}>
        {values.map(({ value, label }, index) => (
          <SettingsValue
            key={label}
            label={label}
            value={value}
            // The first value is primary and the other secondary
            type={index === 0 ? "primary" : "secondary"}
          />
        ))}
      </Stack>
    </section>
  )
}

export const DeploySettingsLayout: React.FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()

  return (
    <Margins>
      <Stack className={styles.wrapper} direction="row" spacing={5}>
        <Sidebar>
          <SidebarNavItem
            href="/settings/deployment/general"
            icon={<SidebarNavItemIcon icon={LaunchOutlined} />}
          >
            Deployment
          </SidebarNavItem>
          <SidebarCaption>Authentication</SidebarCaption>
          <SidebarNavItem
            href="/settings/deployment/auth"
            icon={<SidebarNavItemIcon icon={VpnKeyOutlined} />}
          >
            OAuth
          </SidebarNavItem>
          <SidebarNavItem
            href="/settings/deployment/oidc"
            icon={<SidebarNavItemIcon icon={VpnKeyOutlined} />}
          >
            OIDC
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

  sidebarCaption: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    fontWeight: 600,
    margin: theme.spacing(2, 0, 1.5, 3),
    display: "block",
  },

  content: {
    maxWidth: 800,
    width: "100%",
  },

  headingGroup: {
    marginBottom: theme.spacing(3),
    maxWidth: 360,
  },

  title: {
    fontSize: 36,
    fontWeight: 700,
    display: "flex",
    alignItems: "center",
    lineHeight: "initial",
    margin: 0,
    marginBottom: theme.spacing(2),
  },

  description: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
  },

  enterpriseBadge: {
    fontSize: 10,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: "0.085em",
    marginLeft: theme.spacing(2),
    backgroundColor: theme.palette.success.dark,
    padding: theme.spacing(0.5, 2),
    borderRadius: 9999,
    border: `1px solid ${theme.palette.success.light}`,
    lineHeight: "160%",
  },

  settingsList: {
    background: theme.palette.background.paper,
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
  },

  settingsItem: {
    padding: theme.spacing(4, 5),

    "&:not(:last-child)": {
      borderBottom: `1px solid ${theme.palette.divider}`,
    },
  },

  settingsItemTitle: {
    fontSize: 20,
    fontWeight: 400,
    lineHeight: "initial",
    margin: 0,
    marginBottom: theme.spacing(0.5),
  },

  settingsItemDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
  },

  settingsValues: {
    marginTop: theme.spacing(3),
  },

  settingsValueLabel: {
    fontSize: 14,
    fontWeight: 600,
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(0.5),
    display: "block",
  },

  settingsValueValue: {
    display: "block",
    fontSize: 16,
  },

  settingsValueSecondary: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    color: theme.palette.text.secondary,
  },
}))
