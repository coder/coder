import { fade, makeStyles, Theme } from "@material-ui/core/styles"
import KeyboardArrowDown from "@material-ui/icons/KeyboardArrowDown"
import KeyboardArrowUp from "@material-ui/icons/KeyboardArrowUp"
import { FC } from "react"

const useStyles = makeStyles((theme: Theme) => ({
  arrowIcon: {
    color: fade(theme.palette.primary.contrastText, 0.7),
    marginLeft: theme.spacing(1),
    width: 16,
    height: 16,
  },
  arrowIconUp: {
    color: theme.palette.primary.contrastText,
  },
}))

export const OpenDropdown: FC<React.PropsWithChildren<unknown>> = () => {
  const styles = useStyles()
  return <KeyboardArrowDown className={styles.arrowIcon} />
}

export const CloseDropdown: FC<React.PropsWithChildren<unknown>> = () => {
  const styles = useStyles()
  return <KeyboardArrowUp className={`${styles.arrowIcon} ${styles.arrowIconUp}`} />
}
