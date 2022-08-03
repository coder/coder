import Popover, { PopoverProps } from "@material-ui/core/Popover"
import { fade, makeStyles } from "@material-ui/core/styles"
import { FC } from "react"

type BorderedMenuVariant = "admin-dropdown" | "user-dropdown"

export type BorderedMenuProps = Omit<PopoverProps, "variant"> & {
  variant?: BorderedMenuVariant
}

export const BorderedMenu: FC<PropsWithChildren<BorderedMenuProps>> = ({
  children,
  variant,
  ...rest
}) => {
  const styles = useStyles()

  return (
    <Popover
      classes={{ root: styles.root, paper: styles.paperRoot }}
      data-variant={variant}
      {...rest}
    >
      {children}
    </Popover>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    "&[data-variant='admin-dropdown'] $paperRoot": {
      padding: `${theme.spacing(3)}px 0`,
    },

    "&[data-variant='user-dropdown'] $paperRoot": {
      paddingBottom: theme.spacing(1),
      width: 292,
    },
  },
  paperRoot: {
    width: "292px",
    border: `2px solid ${theme.palette.secondary.dark}`,
    borderRadius: 7,
    boxShadow: `4px 4px 0px ${fade(theme.palette.secondary.dark, 0.2)}`,
  },
}))
