import IconButton from "@material-ui/core/IconButton"
import Menu, { MenuProps } from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import MoreVertIcon from "@material-ui/icons/MoreVert"
import React from "react"
import { UserResponse } from "../../api/types"

export const UserMenu: React.FC<{ user: UserResponse }> = () => {
  const [anchorEl, setAnchorEl] = React.useState<MenuProps["anchorEl"]>(null)

  const handleClick = (event: React.MouseEvent) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  return (
    <>
      <IconButton size="small" aria-label="more" aria-controls="long-menu" aria-haspopup="true" onClick={handleClick}>
        <MoreVertIcon />
      </IconButton>
      <Menu id="simple-menu" anchorEl={anchorEl} keepMounted open={Boolean(anchorEl)} onClose={handleClose}>
        <MenuItem onClick={handleClose}>Suspend</MenuItem>
      </Menu>
    </>
  )
}
