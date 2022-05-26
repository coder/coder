import ListItem from "@material-ui/core/ListItem"
import ListItemText from "@material-ui/core/ListItemText"
import { fade, makeStyles, Theme } from "@material-ui/core/styles"
import React, { useState } from "react"
import { navHeight } from "../../theme/constants"
import { BorderedMenu } from "../BorderedMenu/BorderedMenu"
import { BorderedMenuRow } from "../BorderedMenuRow/BorderedMenuRow"
import { CloseDropdown, OpenDropdown } from "../DropdownArrows/DropdownArrows"
import { UsersOutlinedIcon } from "../Icons/UsersOutlinedIcon"

export const Language = {
  menuTitle: "Admin",
  usersLabel: "Users",
  usersDescription: "Manage users, roles, and permissions.",
}

const entries = [
  {
    label: Language.usersLabel,
    description: Language.usersDescription,
    path: "/users",
    Icon: UsersOutlinedIcon,
  },
]

export const AdminDropdown: React.FC = () => {
  const styles = useStyles()
  const [anchorEl, setAnchorEl] = useState<HTMLElement>()
  const onClose = () => setAnchorEl(undefined)
  const onOpenAdminMenu = (ev: React.MouseEvent<HTMLDivElement>) => setAnchorEl(ev.currentTarget)

  return (
    <>
      <div className={styles.link}>
        <ListItem selected={Boolean(anchorEl)} button onClick={onOpenAdminMenu}>
          <ListItemText className="no-brace" primary={Language.menuTitle} />
          {anchorEl ? <CloseDropdown /> : <OpenDropdown />}
        </ListItem>
      </div>

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
        {entries.map((entry) => (
          <BorderedMenuRow
            description={entry.description}
            Icon={entry.Icon}
            key={entry.label}
            path={entry.path}
            title={entry.label}
            variant="narrow"
            onClick={() => {
              onClose()
            }}
          />
        ))}
      </BorderedMenu>
    </>
  )
}

const useStyles = makeStyles((theme: Theme) => ({
  link: {
    "&:focus": {
      outline: "none",

      "& .MuiListItem-button": {
        background: fade(theme.palette.primary.light, 0.1),
      },
    },

    "& .MuiListItemText-root": {
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
    },
    "& .feature-stage-chip": {
      position: "absolute",
      bottom: theme.spacing(1),

      "& .MuiChip-labelSmall": {
        fontSize: "10px",
      },
    },
    whiteSpace: "nowrap",
    "& .MuiListItem-button": {
      height: navHeight,
      color: "#A7A7A7",
      padding: `0 ${theme.spacing(3)}px`,

      "&.Mui-selected": {
        background: "transparent",
        "& .MuiListItemText-root": {
          color: theme.palette.primary.contrastText,

          "&:not(.no-brace) .MuiTypography-root": {
            position: "relative",

            "&::before": {
              content: `"{"`,
              left: -14,
              position: "absolute",
            },
            "&::after": {
              content: `"}"`,
              position: "absolute",
              right: -14,
            },
          },
        },
      },

      "&.Mui-focusVisible, &:hover": {
        background: "#333",
      },

      "& .MuiListItemText-primary": {
        fontFamily: theme.typography.fontFamily,
        fontSize: 16,
        fontWeight: 500,
      },
    },
  },
}))
