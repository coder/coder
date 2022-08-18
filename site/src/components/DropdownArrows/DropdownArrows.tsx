import { fade, makeStyles, Theme } from "@material-ui/core/styles"
import KeyboardArrowDown from "@material-ui/icons/KeyboardArrowDown"
import KeyboardArrowUp from "@material-ui/icons/KeyboardArrowUp"
import { FC } from "react"

const useStyles = makeStyles((theme: Theme) => ({
  arrowIcon: {
    color: fade(theme.palette.primary.contrastText, 0.7),
    marginLeft: (margin: boolean) => (margin ? theme.spacing(1) : 0),
    width: 16,
    height: 16,
  },
  arrowIconUp: {
    color: theme.palette.primary.contrastText,
  },
}))

interface ArrowProps {
  margin: boolean
}

export const OpenDropdown: FC<ArrowProps> = ({ margin = true }) => {
  const styles = useStyles(margin)
  return <KeyboardArrowDown className={styles.arrowIcon} />
}

export const CloseDropdown: FC<ArrowProps> = ({ margin = true }) => {
  const styles = useStyles(margin)
  return <KeyboardArrowUp className={`${styles.arrowIcon} ${styles.arrowIconUp}`} />
}
