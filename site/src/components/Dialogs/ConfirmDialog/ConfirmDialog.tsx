import DialogActions from "@material-ui/core/DialogActions"
import { makeStyles } from "@material-ui/core/styles"
import { ReactNode, FC, PropsWithChildren } from "react"
import {
  Dialog,
  DialogActionButtons,
  DialogActionButtonsProps,
} from "../Dialog"
import { ConfirmDialogType } from "../types"

interface ConfirmDialogTypeConfig {
  confirmText: ReactNode
  hideCancel: boolean
}

const CONFIRM_DIALOG_DEFAULTS: Record<
  ConfirmDialogType,
  ConfirmDialogTypeConfig
> = {
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
  extends Omit<
    DialogActionButtonsProps,
    "color" | "confirmDialog" | "onCancel"
  > {
  readonly description?: ReactNode
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
      width: "100%",
      maxWidth: theme.spacing(55),
    },
    "& .MuiDialogActions-spacing": {
      padding: `0 ${theme.spacing(5)}px ${theme.spacing(5)}px`,
    },
  },
  dialogContent: {
    color: theme.palette.text.secondary,
    padding: theme.spacing(5),
  },
  dialogTitle: {
    margin: 0,
    marginBottom: theme.spacing(2),
    color: theme.palette.text.primary,
    fontWeight: 400,
    fontSize: theme.spacing(2.5),
  },
  dialogDescription: {
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    fontSize: 16,

    "& strong": {
      color: theme.palette.text.primary,
    },

    "& p": {
      margin: theme.spacing(1, 0),
    },
  },
}))

/**
 * Quick-use version of the Dialog component with slightly alternative styles,
 * great to use for dialogs that don't have any interaction beyond yes / no.
 */
export const ConfirmDialog: FC<PropsWithChildren<ConfirmDialogProps>> = ({
  cancelText,
  confirmLoading,
  confirmText,
  description,
  disabled = false,
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
    <Dialog className={styles.dialogWrapper} onClose={onClose} open={open}>
      <div className={styles.dialogContent}>
        <h3 className={styles.dialogTitle}>{title}</h3>
        {description && (
          <div className={styles.dialogDescription}>{description}</div>
        )}
      </div>

      <DialogActions>
        <DialogActionButtons
          cancelText={cancelText}
          confirmDialog
          confirmLoading={confirmLoading}
          confirmText={confirmText || defaults.confirmText}
          disabled={disabled}
          onCancel={!hideCancel ? onClose : undefined}
          onConfirm={onConfirm || onClose}
          type={type}
        />
      </DialogActions>
    </Dialog>
  )
}
