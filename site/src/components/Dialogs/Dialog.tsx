import MuiDialog, {
  DialogProps as MuiDialogProps,
} from "@material-ui/core/Dialog"
import { alpha, darken, makeStyles } from "@material-ui/core/styles"
import * as React from "react"
import { colors } from "theme/colors"
import { combineClasses } from "../../util/combineClasses"
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
    return "secondary"
  }
  return "primary"
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
  const styles = useButtonStyles({ type })

  return (
    <>
      {onCancel && (
        <LoadingButton
          disabled={confirmLoading}
          onClick={onCancel}
          variant="outlined"
          fullWidth
        >
          {cancelText}
        </LoadingButton>
      )}
      {onConfirm && (
        <LoadingButton
          fullWidth
          variant="contained"
          onClick={onConfirm}
          color={typeToColor(type)}
          loading={confirmLoading}
          disabled={disabled}
          type="submit"
          className={combineClasses({
            [styles.errorButton]: type === "delete",
            [styles.successButton]: type === "success",
          })}
        >
          {confirmText}
        </LoadingButton>
      )}
    </>
  )
}

const useButtonStyles = makeStyles((theme) => ({
  errorButton: {
    "&.MuiButton-contained": {
      backgroundColor: colors.red[10],
      borderColor: colors.red[9],
      color: theme.palette.text.primary,
      "&:hover": {
        backgroundColor: colors.red[9],
      },
      "&.Mui-disabled": {
        opacity: 0.5,
      },
    },
  },
  successButton: {
    "&.MuiButton-contained": {
      backgroundColor: theme.palette.success.main,
      color: theme.palette.primary.contrastText,
      "&:hover": {
        backgroundColor: darken(theme.palette.success.main, 0.3),
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
        "&.Mui-disabled": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        backgroundColor: theme.palette.action.disabledBackground,
        color: alpha(theme.palette.text.disabled, 0.5),
      },
    },

    "&.MuiButton-outlined": {
      color: theme.palette.success.main,
      borderColor: theme.palette.success.main,
      "&:hover": {
        backgroundColor: alpha(
          theme.palette.success.main,
          theme.palette.action.hoverOpacity,
        ),
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
        "&.Mui-disabled": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: alpha(theme.palette.text.disabled, 0.5),
        borderColor: theme.palette.action.disabled,
      },
    },

    "&.MuiButton-text": {
      color: theme.palette.success.main,
      "&:hover": {
        backgroundColor: alpha(
          theme.palette.success.main,
          theme.palette.action.hoverOpacity,
        ),
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: alpha(theme.palette.text.disabled, 0.5),
      },
    },
  },
}))

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
