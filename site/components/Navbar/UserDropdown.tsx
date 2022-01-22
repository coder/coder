import Avatar from "@material-ui/core/Avatar"
import Badge from "@material-ui/core/Badge"
import ListItemIcon from "@material-ui/core/ListItemIcon"
import ListItemText from "@material-ui/core/ListItemText"
import MenuItem from "@material-ui/core/MenuItem"
import { fade, makeStyles } from "@material-ui/core/styles"
//import AccountIcon from "@material-ui/icons/AccountCircleOutlined"
import KeyboardArrowDown from "@material-ui/icons/KeyboardArrowDown"
import KeyboardArrowUp from "@material-ui/icons/KeyboardArrowUp"
import React, { useState } from "react"
import { LogoutIcon } from "../Icons"
import { BorderedMenu } from "./BorderedMenu"
import { UserProfileCard } from "../User/UserProfileCard"

import { User } from "../../contexts/UserContext"
import Divider from "@material-ui/core/Divider"

const navHeight = 56

export interface UserDropdownProps {
  user: User
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

  // TODO: what does this do?
  const isSelected = false

  return (
    <>
      <div>
        <MenuItem onClick={handleDropdownClick} selected={isSelected}>
          <div className={styles.inner}>
            {user && (
              <Badge overlap="circle">
                <Avatar>T</Avatar>
              </Badge>
            )}
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
        {user && (
          <div className={styles.userInfo}>
            <UserProfileCard user={user} onAvatarClick={onPopoverClose} />

            <Divider className={styles.divider} />

            <MenuItem className={styles.menuItem} onClick={onSignOut}>
              <ListItemIcon className={styles.icon}>
                <LogoutIcon />
              </ListItemIcon>
              <ListItemText primary="Sign Out" />
            </MenuItem>
          </div>
        )}
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

  icon: {
    color: theme.palette.text.secondary,
  },
}))
