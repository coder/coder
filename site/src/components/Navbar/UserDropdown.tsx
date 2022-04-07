import Badge from "@material-ui/core/Badge"
import Divider from "@material-ui/core/Divider"
import ListItemIcon from "@material-ui/core/ListItemIcon"
import ListItemText from "@material-ui/core/ListItemText"
import MenuItem from "@material-ui/core/MenuItem"
import { fade, makeStyles } from "@material-ui/core/styles"
import AccountIcon from "@material-ui/icons/AccountCircleOutlined"
import KeyboardArrowDown from "@material-ui/icons/KeyboardArrowDown"
import KeyboardArrowUp from "@material-ui/icons/KeyboardArrowUp"
import React, { useState } from "react"
import { Link } from "react-router-dom"
import { UserResponse } from "../../api/types"
import { LogoutIcon } from "../Icons"
import { DocsIcon } from "../Icons/DocsIcon"
import { UserAvatar } from "../User"
import { UserProfileCard } from "../User/UserProfileCard"
import { BorderedMenu } from "./BorderedMenu"

export const Language = {
  accountLabel: "Account",
  docsLabel: "Documentation",
  signOutLabel: "Sign Out",
}
export interface UserDropdownProps {
  user: UserResponse
  onSignOut: () => void
}

export const UserDropdown: React.FC<UserDropdownProps> = ({ user, onSignOut }: UserDropdownProps) => {
  const styles = useStyles()

  const [anchorEl, setAnchorEl] = useState<HTMLElement | undefined>()
  const handleDropdownClick = (ev: React.MouseEvent<HTMLLIElement>): void => {
    setAnchorEl(ev.currentTarget)
  }
  const onPopoverClose = () => {
    setAnchorEl(undefined)
  }

  return (
    <>
      <div>
        <MenuItem onClick={handleDropdownClick} data-testid="user-dropdown-trigger">
          <div className={styles.inner}>
            <Badge overlap="circle">
              <UserAvatar username={user.username} />
            </Badge>
            {anchorEl ? (
              <KeyboardArrowUp className={`${styles.arrowIcon} ${styles.arrowIconUp}`} />
            ) : (
              <KeyboardArrowDown className={styles.arrowIcon} />
            )}
          </div>
        </MenuItem>
      </div>

      <BorderedMenu
        anchorEl={anchorEl}
        getContentAnchorEl={null}
        open={!!anchorEl}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
        marginThreshold={0}
        variant="user-dropdown"
        onClose={onPopoverClose}
      >
        <div className={styles.userInfo}>
          <UserProfileCard user={user} />

          <Divider />

          <Link to="/preferences" className={styles.link}>
            <MenuItem className={styles.menuItem} onClick={handleDropdownClick}>
              <ListItemIcon className={styles.icon}>
                <AccountIcon />
              </ListItemIcon>
              <ListItemText primary={Language.accountLabel} />
            </MenuItem>
          </Link>

          <a href="https://coder.com/docs" target="_blank" rel="noreferrer" className={styles.link}>
            <MenuItem className={styles.menuItem} onClick={handleDropdownClick}>
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
      </BorderedMenu>
    </>
  )
}

export const useStyles = makeStyles((theme) => ({
  divider: {
    marginTop: theme.spacing(1),
    marginBottom: theme.spacing(1),
  },

  inner: {
    display: "flex",
    alignItems: "center",
    minWidth: 0,
    maxWidth: 300,
  },

  userInfo: {
    marginBottom: theme.spacing(1),
  },

  arrowIcon: {
    color: fade(theme.palette.primary.contrastText, 0.7),
    marginLeft: theme.spacing(1),
    width: 16,
    height: 16,
  },

  arrowIconUp: {
    color: theme.palette.primary.contrastText,
  },

  menuItem: {
    height: 44,
    padding: `${theme.spacing(1.5)}px ${theme.spacing(2.75)}px`,

    "&:hover": {
      backgroundColor: fade(theme.palette.primary.light, 0.1),
      transition: "background-color 0.3s ease",
    },
  },

  link: {
    textDecoration: "none",
    color: "inherit",
  },

  icon: {
    color: theme.palette.text.secondary,
  },
}))
