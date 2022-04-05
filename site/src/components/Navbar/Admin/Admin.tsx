import React from "react"
import { useNavigate } from "react-router-dom"
import { BorderedMenu } from "../../BorderedMenu"
import { BorderedMenuRow } from "../../BorderedMenu/BorderedMenuRow"
import { NavbarEntryProps } from "../../BorderedMenu/types"

interface AdminDropdownProps {
  anchorEl?: HTMLElement
  entries: NavbarEntryProps[]
  onClose: () => void
}

export const AdminDropdown: React.FC<AdminDropdownProps> = ({ anchorEl, entries, onClose }) => {
  const navigate = useNavigate()

  return (
    <BorderedMenu
      anchorEl={anchorEl}
      getContentAnchorEl={null}
      open={!!anchorEl}
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "center",
      }}
      transformOrigin={{
        vertical: "top",
        horizontal: "center",
      }}
      marginThreshold={0}
      variant="admin-dropdown"
      onClose={onClose}
    >
      {entries.map((entry) =>
        entry.label && entry.Icon ? (
          <BorderedMenuRow
            description={entry.description}
            Icon={entry.Icon}
            key={entry.label}
            path={entry.path}
            title={entry.label}
            variant="narrow"
            onClick={() => {
              onClose()
              navigate(entry.path)
            }}
          />
        ) : null,
      )}
    </BorderedMenu>
  )
}
