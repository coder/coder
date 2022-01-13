import ListItemIcon from "@material-ui/core/ListItemIcon"
import MenuItem from "@material-ui/core/MenuItem"
import { SvgIcon, Typography } from "@material-ui/core"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"

export interface NavMenuEntryProps {
  icon: typeof SvgIcon
  path: string
  label?: string
  selected: boolean
  className?: string
  onClick?: () => void
}

export const NavMenuEntry: React.FC<NavMenuEntryProps> = ({
  className,
  icon,
  path,
  label = path,
  selected,
  onClick,
}) => {
  const styles = useStyles()
  const Icon = icon
  return (
    <MenuItem selected={selected} className={className} onClick={onClick}>
      <div className={styles.root}>
        {icon && (
          <ListItemIcon>
            <Icon className={styles.icon} />
          </ListItemIcon>
        )}
        <Typography>{label}</Typography>
      </div>
    </MenuItem>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    padding: "2em",
  },
  icon: {
    color: theme.palette.text.primary,

    "& path": {
      fill: theme.palette.text.primary,
    },
  },
}))
