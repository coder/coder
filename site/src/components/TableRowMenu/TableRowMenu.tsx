import IconButton from "@material-ui/core/IconButton"
import Menu, { MenuProps } from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import MoreVertIcon from "@material-ui/icons/MoreVert"
import React from "react"

export interface TableRowMenuProps<TData> {
  data: TData
  menuItems: Array<{
    label: string
    onClick: (data: TData) => void
  }>
}

export const TableRowMenu = <T,>({ data, menuItems }: TableRowMenuProps<T>): JSX.Element => {
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
        {menuItems.map((item) => (
          <MenuItem
            key={item.label}
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
