import Badge from "@material-ui/core/Badge"
import MenuItem from "@material-ui/core/MenuItem"
import { fade, makeStyles } from "@material-ui/core/styles"
import React, { useState } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { navHeight } from "../../theme/constants"
import { BorderedMenu } from "../BorderedMenu/BorderedMenu"
import { CloseDropdown, OpenDropdown } from "../DropdownArrows/DropdownArrows"
import { UserAvatar } from "../UserAvatar/UserAvatar"
import { UserDropdownContent } from "../UserDropdownContent/UserDropdownContent"

export interface UserDropdownProps {
  user: TypesGen.User
  onSignOut: () => void
}

export const UserDropdown: React.FC<React.PropsWithChildren<UserDropdownProps>> = ({
  user,
  onSignOut,
}: UserDropdownProps) => {
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
      <MenuItem
        className={styles.menuItem}
        onClick={handleDropdownClick}
        data-testid="user-dropdown-trigger"
      >
        <div className={styles.inner}>
          <Badge overlap="circle">
            <UserAvatar username={user.username} />
          </Badge>
          {anchorEl ? <CloseDropdown /> : <OpenDropdown />}
        </div>
      </MenuItem>

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
        <UserDropdownContent user={user} onPopoverClose={onPopoverClose} onSignOut={onSignOut} />
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

  menuItem: {
    height: navHeight,
    padding: `${theme.spacing(1.5)}px ${theme.spacing(2.75)}px`,

    "&:hover": {
      backgroundColor: fade(theme.palette.primary.light, 0.05),
      transition: "background-color 0.3s ease",
    },
  },
}))
