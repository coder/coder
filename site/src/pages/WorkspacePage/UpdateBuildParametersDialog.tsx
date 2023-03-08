import Button from "@material-ui/core/Button"
import Dialog from "@material-ui/core/Dialog"
import DialogActions from "@material-ui/core/DialogActions"
import DialogContent from "@material-ui/core/DialogContent"
import DialogContentText from "@material-ui/core/DialogContentText"
import DialogTitle from "@material-ui/core/DialogTitle"
import TextField from "@material-ui/core/TextField"
import { DialogProps } from "components/Dialogs/Dialog"
import { FC } from "react"

export const UpdateBuildParametersDialog: FC<DialogProps> = (props) => {
  return (
    <Dialog {...props} aria-labelledby="update-build-parameters-title">
      <DialogTitle id="update-build-parameters-title">Subscribe</DialogTitle>
      <DialogContent>
        <DialogContentText>
          To subscribe to this website, please enter your email address here. We
          will send updates occasionally.
        </DialogContentText>
        <TextField
          autoFocus
          margin="dense"
          id="name"
          label="Email Address"
          type="email"
          fullWidth
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={() => {}} color="primary">
          Subscribe
        </Button>
      </DialogActions>
    </Dialog>
  )
}
