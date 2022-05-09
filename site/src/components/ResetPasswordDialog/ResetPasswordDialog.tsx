import DialogActions from "@material-ui/core/DialogActions"
import DialogContent from "@material-ui/core/DialogContent"
import DialogContentText from "@material-ui/core/DialogContentText"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { CodeBlock } from "../CodeBlock/CodeBlock"
import { Dialog, DialogActionButtons, DialogTitle } from "../Dialog/Dialog"

export interface ResetPasswordDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  user?: TypesGen.User
  newPassword?: string
  loading: boolean
}

export const Language = {
  title: "Reset password",
  message: (username?: string): JSX.Element => (
    <>
      You will need to send <strong>{username}</strong> the following password:
    </>
  ),
  confirmText: "Reset password",
}

export const ResetPasswordDialog: React.FC<ResetPasswordDialogProps> = ({
  open,
  onClose,
  onConfirm,
  user,
  newPassword,
  loading,
}) => {
  const styles = useStyles()

  return (
    <Dialog open={open} onClose={onClose}>
      <DialogTitle title={Language.title} />

      <DialogContent>
        <DialogContentText variant="subtitle2">{Language.message(user?.username)}</DialogContentText>

        <DialogContentText component="div">
          <CodeBlock lines={[newPassword ?? ""]} className={styles.codeBlock} />
        </DialogContentText>
      </DialogContent>

      <DialogActions>
        <DialogActionButtons
          onCancel={onClose}
          confirmText={Language.confirmText}
          onConfirm={onConfirm}
          confirmLoading={loading}
        />
      </DialogActions>
    </Dialog>
  )
}

const useStyles = makeStyles(() => ({
  codeBlock: {
    minHeight: "auto",
    userSelect: "all",
    width: "100%",
  },
}))
