import { makeStyles, Theme } from "@material-ui/core/styles"
import KeyboardArrowDown from "@material-ui/icons/KeyboardArrowDown"
import KeyboardArrowUp from "@material-ui/icons/KeyboardArrowUp"
import { FC } from "react"

const useStyles = makeStyles<Theme, ArrowProps>((theme: Theme) => ({
  arrowIcon: {
    color: ({ color }) => color ?? theme.palette.primary.contrastText,
    marginLeft: ({ margin }) => (margin ? theme.spacing(1) : 0),
    width: 16,
    height: 16,
  },
  arrowIconUp: {
    color: ({ color }) => color ?? theme.palette.primary.contrastText,
  },
}))

interface ArrowProps {
  margin?: boolean
  color?: string
}

export const OpenDropdown: FC<ArrowProps> = ({ margin = true, color }) => {
  const styles = useStyles({ margin, color })
  return (
    <KeyboardArrowDown
      aria-label="open-dropdown"
      className={styles.arrowIcon}
    />
  )
}

export const CloseDropdown: FC<ArrowProps> = ({ margin = true, color }) => {
  const styles = useStyles({ margin, color })
  return (
    <KeyboardArrowUp
      aria-label="close-dropdown"
      className={`${styles.arrowIcon} ${styles.arrowIconUp}`}
    />
  )
}
