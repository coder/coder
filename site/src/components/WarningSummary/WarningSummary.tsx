import { FC, useState } from "react"
import { Stack } from "components/Stack/Stack"
import { makeStyles, Theme } from "@material-ui/core/styles"
import CloseIcon from "@material-ui/icons/Close"
import ReportProblemOutlinedIcon from "@material-ui/icons/ReportProblemOutlined"
import IconButton from "@material-ui/core/IconButton"
import { colors } from "theme/colors"

export interface WarningSummaryProps {
  warningString: string
}

export const WarningSummary: FC<WarningSummaryProps> = ({ warningString }) => {
  const styles = useStyles()
  const [isOpen, setOpen] = useState(true)

  const closeWarning = () => {
    setOpen(false)
  }

  if (!isOpen) {
    return null
  }

  return (
    <Stack
      className={styles.root}
      direction="row"
      alignItems="center"
      justifyContent="space-between"
    >
      <Stack direction="row" spacing={0} alignItems="center">
        <ReportProblemOutlinedIcon />
        <span className={styles.errorMessage}>{warningString}</span>
      </Stack>

      <IconButton onClick={closeWarning} className={styles.iconButton}>
        <CloseIcon className={styles.closeIcon} />
      </IconButton>
    </Stack>
  )
}
const useStyles = makeStyles<Theme>((theme) => ({
  root: {
    border: `2px solid ${colors.orange[7]}`,
    padding: `8px`,
    borderRadius: theme.shape.borderRadius,
    gap: 0,
    color: `${colors.orange[7]}`, // icon inherits color from parent
  },
  errorMessage: {
    marginRight: `${theme.spacing(1)}px`,
    marginLeft: "10px",
    color: `${colors.orange[4]}`,
  },
  iconButton: {
    padding: 0,
  },
  closeIcon: {
    width: 25,
    height: 25,
    color: `${colors.orange[7]}`,
  },
}))
