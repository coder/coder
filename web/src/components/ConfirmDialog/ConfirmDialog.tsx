import DialogActions from "@material-ui/core/DialogActions"
import { fade, makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React, { ReactNode } from "react"
import { Dialog, DialogActionButtons, DialogActionButtonsProps } from "../Dialog/Dialog"
import { ConfirmDialogType } from "../Dialog/types"

interface ConfirmDialogTypeConfig {
  confirmText: ReactNode
  hideCancel: boolean
}

const CONFIRM_DIALOG_DEFAULTS: Record<ConfirmDialogType, ConfirmDialogTypeConfig> = {
  delete: {
    confirmText: "Delete",
    hideCancel: false,
  },
  info: {
    confirmText: "OK",
    hideCancel: true,
  },
  success: {
    confirmText: "OK",
    hideCancel: true,
  },
}

export interface ConfirmDialogProps
  extends Omit<DialogActionButtonsProps, "color" | "confirmDialog" | "onCancel"> {
  readonly description?: React.ReactNode
  /**
   * hideCancel hides the cancel button when set true, and shows the cancel
   * button when set to false. When undefined:
   *   - cancel is not displayed for "info" dialogs
   *   - cancel is displayed for "delete" dialogs
   */
  readonly hideCancel?: boolean
  /**
   * onClose is called when canceling (if cancel is showing).
   *
   * Additionally, if onConfirm is not defined onClose will be used in its place
   * when confirming.
   */
  readonly onClose: () => void
  readonly open: boolean
  readonly title: string
}

const useStyles = makeStyles((theme) => ({
  dialogWrapper: {
    "& .MuiPaper-root": {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
    },
    "& .MuiDialogActions-spacing": {
      padding: `0 ${theme.spacing(3.75)}px ${theme.spacing(3.75)}px`,
    },
  },
  dialogContent: {
    color: theme.palette.text.secondary,
    padding: theme.spacing(6),
    textAlign: "center",
  },
  titleText: {
    marginBottom: theme.spacing(3),
  },
  description: {
    color: fade(theme.palette.text.secondary, 0.75),
    lineHeight: "160%",

    "& strong": {
      color: fade(theme.palette.text.secondary, 0.95),
    },
  },
}))

/**
 * Quick-use version of the Dialog component with slightly alternative styles,
 * great to use for dialogs that don't have any interaction beyond yes / no.
 */
export const ConfirmDialog: React.FC<React.PropsWithChildren<ConfirmDialogProps>> = ({
  cancelText,
  confirmLoading,
  confirmText,
  description,
  hideCancel,
  onClose,
  onConfirm,
  open = false,
  title,
  type = "info",
}) => {
  const styles = useStyles({ type })

  const defaults = CONFIRM_DIALOG_DEFAULTS[type]

  if (typeof hideCancel === "undefined") {
    hideCancel = defaults.hideCancel
  }

  return (
    <Dialog className={styles.dialogWrapper} maxWidth="sm" onClose={onClose} open={open}>
      <div className={styles.dialogContent}>
        <Typography className={styles.titleText} variant="h3">
          {title}
        </Typography>

        {description && (
          <Typography
            component={typeof description === "string" ? "p" : "div"}
            className={styles.description}
            variant="body2"
          >
            {description}
          </Typography>
        )}
      </div>

      <DialogActions>
        <DialogActionButtons
          cancelText={cancelText}
          confirmDialog
          confirmLoading={confirmLoading}
          confirmText={confirmText || defaults.confirmText}
          onCancel={!hideCancel ? onClose : undefined}
          onConfirm={onConfirm || onClose}
          type={type}
        />
      </DialogActions>
    </Dialog>
  )
}
