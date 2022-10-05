import { useState, FC, ReactElement } from "react"
import Alert from "@material-ui/lab/Alert"
import IconButton from "@material-ui/core/IconButton"
import Collapse from "@material-ui/core/Collapse"
import { Stack } from "components/Stack/Stack"
import CloseIcon from "@material-ui/icons/Close"

export interface WarningAlertProps {
  text: string
  action?: ReactElement
}

export const WarningAlert: FC<WarningAlertProps> = ({ text, action }) => {
  const [open, setOpen] = useState(true)

  return (
    <Stack>
      <Collapse in={open}>
        <Alert
          severity="warning"
          action={
            action ? (
              action
            ) : (
              <IconButton
                aria-label="close"
                color="inherit"
                size="small"
                onClick={() => {
                  setOpen(false)
                }}
              >
                <CloseIcon fontSize="inherit" />
              </IconButton>
            )
          }
        >
          {text}
        </Alert>
      </Collapse>
    </Stack>
  )
}
