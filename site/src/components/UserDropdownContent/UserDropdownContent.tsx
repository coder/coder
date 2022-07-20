import Chip from "@material-ui/core/Chip"
import Divider from "@material-ui/core/Divider"
import ListItemIcon from "@material-ui/core/ListItemIcon"
import ListItemText from "@material-ui/core/ListItemText"
import MenuItem from "@material-ui/core/MenuItem"
import { fade, makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import AccountIcon from "@material-ui/icons/AccountCircleOutlined"
import { FC } from "react"
import { Link } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { Role } from "../../api/typesGenerated"
import { navHeight } from "../../theme/constants"
import { DocsIcon } from "../Icons/DocsIcon"
import { LogoutIcon } from "../Icons/LogoutIcon"
import { UserAvatar } from "../UserAvatar/UserAvatar"

export const Language = {
  accountLabel: "Account",
  docsLabel: "Documentation",
  signOutLabel: "Sign Out",
}

export interface UserDropdownContentProps {
  user: TypesGen.User
  onPopoverClose: () => void
  onSignOut: () => void
}

export const UserDropdownContent: FC<UserDropdownContentProps> = ({
  user,
  onPopoverClose,
  onSignOut,
}) => {
  const styles = useStyles()

  return (
    <div className={styles.userInfo}>
      <div className={styles.root}>
        <div className={styles.avatarContainer}>
          <UserAvatar className={styles.avatar} username={user.username} />
        </div>
        <Typography className={styles.userName}>{user.username}</Typography>
        <Typography className={styles.userEmail}>{user.email}</Typography>
        <ul className={styles.chipContainer}>
          {user.roles.map((role: Role) => (
            <li key={role.name} className={styles.chipStyles}>
              <Chip classes={{ root: styles.chipRoot }} label={role.display_name} />
            </li>
          ))}
        </ul>
      </div>

      <Divider />

      <Link to="/settings/account" className={styles.link}>
        <MenuItem className={styles.menuItem} onClick={onPopoverClose}>
          <ListItemIcon className={styles.icon}>
            <AccountIcon />
          </ListItemIcon>
          <ListItemText primary={Language.accountLabel} />
        </MenuItem>
      </Link>

      <Divider />

      <a
        href={`https://github.com/coder/coder/tree/${process.env.CODER_VERSION}/docs`}
        target="_blank"
        rel="noreferrer"
        className={styles.link}
      >
        <MenuItem className={styles.menuItem} onClick={onPopoverClose}>
          <ListItemIcon className={styles.icon}>
            <DocsIcon />
          </ListItemIcon>
          <ListItemText primary={Language.docsLabel} />
        </MenuItem>
      </a>

      <MenuItem className={styles.menuItem} onClick={onSignOut}>
        <ListItemIcon className={styles.icon}>
          <LogoutIcon />
        </ListItemIcon>
        <ListItemText primary={Language.signOutLabel} />
      </MenuItem>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    paddingTop: theme.spacing(3),
    textAlign: "center",
  },
  avatarContainer: {
    width: "100%",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
  },
  avatar: {
    width: 48,
    height: 48,
    borderRadius: "50%",
    marginBottom: theme.spacing(1),
    transition: `transform .2s`,

    "&:hover": {
      transform: `scale(1.1)`,
    },
  },
  userName: {
    fontSize: 16,
    marginBottom: theme.spacing(0.5),
  },
  userEmail: {
    fontSize: 14,
    letterSpacing: 0.2,
    color: theme.palette.text.secondary,
  },
  chipContainer: {
    display: "flex",
    justifyContent: "center",
    flexWrap: "wrap",
    listStyle: "none",
    margin: "0",
    padding: `${theme.spacing(1.5)}px ${theme.spacing(2.75)}px`,
  },
  chipStyles: {
    margin: theme.spacing(0.5),
  },
  chipRoot: {
    backgroundColor: theme.palette.secondary.dark,
  },
  link: {
    textDecoration: "none",
    color: "inherit",
  },
  menuItem: {
    height: navHeight,
    padding: `${theme.spacing(1.5)}px ${theme.spacing(2.75)}px`,

    "&:hover": {
      backgroundColor: fade(theme.palette.primary.light, 0.05),
      transition: "background-color 0.3s ease",
    },
  },
  userInfo: {
    marginBottom: theme.spacing(1),
  },
  icon: {
    color: theme.palette.text.secondary,
  },
}))
