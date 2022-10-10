import Popover, { PopoverProps } from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import { FC, PropsWithChildren } from "react"

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
      minWidth: 292,
    },
  },
  paperRoot: {
    minWidth: 292,
    border: `2px solid ${theme.palette.secondary.dark}`,
    borderRadius: theme.shape.borderRadius,
    boxShadow: theme.shadows[6],
  },
}))
