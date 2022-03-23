import Badge from "@material-ui/core/Badge"
import Divider from "@material-ui/core/Divider"
import ListItemIcon from "@material-ui/core/ListItemIcon"
import ListItemText from "@material-ui/core/ListItemText"
import MenuItem from "@material-ui/core/MenuItem"
import { fade, makeStyles } from "@material-ui/core/styles"
import KeyboardArrowDown from "@material-ui/icons/KeyboardArrowDown"
import KeyboardArrowUp from "@material-ui/icons/KeyboardArrowUp"
import React, { useState } from "react"
import { LogoutIcon } from "../Icons"
import { BorderedMenu } from "./BorderedMenu"
import { UserProfileCard } from "../User/UserProfileCard"

import { UserAvatar } from "../User"
import { UserResponse } from "../../api/types"

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
        <MenuItem onClick={handleDropdownClick}>
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

          <Divider className={styles.divider} />

          <MenuItem className={styles.menuItem} onClick={onSignOut}>
            <ListItemIcon className={styles.icon}>
              <LogoutIcon />
            </ListItemIcon>
            <ListItemText primary="Sign Out" />
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

  icon: {
    color: theme.palette.text.secondary,
  },
}))
