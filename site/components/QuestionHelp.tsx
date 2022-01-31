import { makeStyles } from "@material-ui/core/styles"
import HelpIcon from "@material-ui/icons/Help"
import * as React from "react"

export const QuestionHelp: React.FC = () => {
  const styles = useStyles()
  return (
    <HelpIcon className={styles.icon} />
  )
}

const useStyles = makeStyles((theme) => ({
  icon: {
    display: "block",
    height: 20,
    width: 20,
    color: theme.palette.text.secondary,
    opacity: 0.5,
    "&:hover": {
      opacity: 1,
    },
  },
}))
