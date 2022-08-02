import IconButton from "@material-ui/core/IconButton"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React, { useEffect } from "react"
import { CloseIcon } from "../Icons/CloseIcon"

export interface FormCloseButtonProps {
  onClose: () => void
}

export const FormCloseButton: React.FC<React.PropsWithChildren<FormCloseButtonProps>> = ({
  onClose,
}) => {
  const styles = useStyles()

  useEffect(() => {
    const handleKeyPress = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose()
      }
    }

    document.body.addEventListener("keydown", handleKeyPress, false)

    return () => {
      document.body.removeEventListener("keydown", handleKeyPress, false)
    }
  }, [onClose])

  return (
    <IconButton className={styles.closeButton} onClick={onClose} size="medium">
      <CloseIcon />
      <Typography variant="caption" className={styles.label}>
        ESC
      </Typography>
    </IconButton>
  )
}

const useStyles = makeStyles((theme) => ({
  closeButton: {
    position: "fixed",
    top: theme.spacing(3),
    right: theme.spacing(6),
    opacity: 0.5,
    color: theme.palette.text.primary,
    "&:hover": {
      opacity: 1,
    },
  },
  label: {
    position: "absolute",
    left: "50%",
    top: "100%",
    transform: "translate(-50%, 50%)",
  },
}))
