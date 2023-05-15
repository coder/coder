import MuiDialog, { DialogProps as MuiDialogProps } from "@mui/material/Dialog"
import * as React from "react"
import {
  LoadingButton,
  LoadingButtonProps,
} from "../LoadingButton/LoadingButton"
import { ConfirmDialogType } from "./types"

export interface DialogActionButtonsProps {
  /** Text to display in the cancel button */
  cancelText?: string
  /** Text to display in the confirm button */
  confirmText?: React.ReactNode
  /** Whether or not confirm is loading, also disables cancel when true */
  confirmLoading?: boolean
  /** Whether or not this is a confirm dialog */
  confirmDialog?: boolean
  /** Whether or not the submit button is disabled */
  disabled?: boolean
  /** Called when cancel is clicked */
  onCancel?: () => void
  /** Called when confirm is clicked */
  onConfirm?: () => void
  type?: ConfirmDialogType
}

const typeToColor = (type: ConfirmDialogType): LoadingButtonProps["color"] => {
  if (type === "delete") {
    return "error"
  }

  if (type === "success") {
    return "success"
  }
}

/**
 * Quickly handles most modals actions, some combination of a cancel and confirm button
 */
export const DialogActionButtons: React.FC<DialogActionButtonsProps> = ({
  cancelText = "Cancel",
  confirmText = "Confirm",
  confirmLoading = false,
  disabled = false,
  onCancel,
  onConfirm,
  type = "info",
}) => {
  return (
    <>
      {onCancel && (
        <LoadingButton disabled={confirmLoading} onClick={onCancel} fullWidth>
          {cancelText}
        </LoadingButton>
      )}
      {onConfirm && (
        <LoadingButton
          fullWidth
          data-testid="confirm-button"
          variant="contained"
          onClick={onConfirm}
          color={typeToColor(type)}
          loading={confirmLoading}
          disabled={disabled}
          type="submit"
        >
          {confirmText}
        </LoadingButton>
      )}
    </>
  )
}

export type DialogProps = MuiDialogProps

/**
 * Wrapper around Material UI's Dialog component. Conveniently exports all of
 * Dialog's components in one import, so for example `<DialogContent />` becomes
 * `<Dialog.Content />` etc. Also contains some custom Dialog components listed below.
 *
 * See original component's Material UI documentation here: https://material-ui.com/components/dialogs/
 */
export const Dialog: React.FC<DialogProps> = (props) => {
  // Wrapped so we can add custom attributes below
  return <MuiDialog {...props} />
}
