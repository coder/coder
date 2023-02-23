import IconButton from "@material-ui/core/IconButton"
import Menu, { MenuProps } from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import MoreVertIcon from "@material-ui/icons/MoreVert"
import { MouseEvent, useState } from "react"

export interface TableRowMenuProps<TData> {
  data: TData
  menuItems: Array<{
    label: string
    disabled: boolean
    onClick: (data: TData) => void
  }>
}

export const TableRowMenu = <T,>({
  data,
  menuItems,
}: TableRowMenuProps<T>): JSX.Element => {
  const [anchorEl, setAnchorEl] = useState<MenuProps["anchorEl"]>(null)

  const handleClick = (event: MouseEvent) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  return (
    <>
      <IconButton
        size="small"
        aria-label="more"
        aria-controls="long-menu"
        aria-haspopup="true"
        onClick={handleClick}
      >
        <MoreVertIcon />
      </IconButton>
      <Menu
        id="simple-menu"
        anchorEl={anchorEl}
        keepMounted
        open={Boolean(anchorEl)}
        onClose={handleClose}
      >
        {menuItems.map((item) => (
          <MenuItem
            key={item.label}
            disabled={item.disabled}
            onClick={() => {
              handleClose()
              item.onClick(data)
            }}
          >
            {item.label}
          </MenuItem>
        ))}
      </Menu>
    </>
  )
}
