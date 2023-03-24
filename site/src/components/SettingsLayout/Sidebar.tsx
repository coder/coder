import { makeStyles } from "@material-ui/core/styles"
import VpnKeyOutlined from "@material-ui/icons/VpnKeyOutlined"
import FingerprintOutlinedIcon from "@material-ui/icons/FingerprintOutlined"
import { User } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { FC, ElementType, PropsWithChildren, ReactNode } from "react"
import { NavLink } from "react-router-dom"
import { combineClasses } from "util/combineClasses"
import AccountIcon from "@material-ui/icons/Person"
import SecurityIcon from "@material-ui/icons/LockOutlined"

const SidebarNavItem: FC<
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

export const Sidebar: React.FC<{ user: User }> = ({ user }) => {
  const styles = useStyles()

  return (
    <nav className={styles.sidebar}>
      <Stack direction="row" alignItems="center" className={styles.userInfo}>
        <UserAvatar username={user.username} avatarURL={user.avatar_url} />
        <Stack spacing={0} className={styles.userData}>
          <span className={styles.username}>{user.username}</span>
          <span className={styles.email}>{user.email}</span>
        </Stack>
      </Stack>

      <SidebarNavItem
        href="account"
        icon={<SidebarNavItemIcon icon={AccountIcon} />}
      >
        Account
      </SidebarNavItem>
      <SidebarNavItem
        href="security"
        icon={<SidebarNavItemIcon icon={SecurityIcon} />}
      >
        Security
      </SidebarNavItem>
      <SidebarNavItem
        href="ssh-keys"
        icon={<SidebarNavItemIcon icon={FingerprintOutlinedIcon} />}
      >
        SSH Keys
      </SidebarNavItem>
      <SidebarNavItem
        href="tokens"
        icon={<SidebarNavItemIcon icon={VpnKeyOutlined} />}
      >
        Tokens
      </SidebarNavItem>
    </nav>
  )
}

const useStyles = makeStyles((theme) => ({
  sidebar: {
    width: 245,
    flexShrink: 0,
  },
  sidebarNavItem: {
    color: "inherit",
    display: "block",
    fontSize: 14,
    textDecoration: "none",
    padding: theme.spacing(1.5, 1.5, 1.5, 2),
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
      borderTopLeftRadius: theme.shape.borderRadius,
      borderBottomLeftRadius: theme.shape.borderRadius,
    },
  },
  sidebarNavItemIcon: {
    width: theme.spacing(2),
    height: theme.spacing(2),
  },
  userInfo: {
    marginBottom: theme.spacing(2),
  },
  userData: {
    overflow: "hidden",
  },
  username: {
    fontWeight: 600,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  email: {
    color: theme.palette.text.secondary,
    fontSize: 12,
    overflow: "hidden",
    textOverflow: "ellipsis",
  },
}))
