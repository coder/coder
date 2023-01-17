import MuiDialog, {
  DialogProps as MuiDialogProps,
} from "@material-ui/core/Dialog"
import { alpha, darken, lighten, makeStyles } from "@material-ui/core/styles"
import * as React from "react"
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
  confirmDialog,
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
          className={combineClasses({
            [styles.dialogButton]: true,
            [styles.cancelButton]: true,
            [styles.confirmDialogCancelButton]: confirmDialog,
          })}
          disabled={confirmLoading}
          onClick={onCancel}
          variant="outlined"
        >
          {cancelText}
        </LoadingButton>
      )}
      {onConfirm && (
        <LoadingButton
          variant="contained"
          onClick={onConfirm}
          color={typeToColor(type)}
          loading={confirmLoading}
          disabled={disabled}
          type="submit"
          className={combineClasses({
            [styles.dialogButton]: true,
            [styles.submitButton]: true,
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

interface StyleProps {
  type: ConfirmDialogType
}

const useButtonStyles = makeStyles((theme) => ({
  dialogButton: {
    borderRadius: theme.shape.borderRadius,
    fontSize: theme.typography.h6.fontSize,
    fontWeight: theme.typography.h5.fontWeight,
    padding: `${theme.spacing(0.75)}px ${theme.spacing(2)}px`,
    width: "100%",
    boxShadow: "none",
  },
  cancelButton: {
    background: alpha(theme.palette.primary.main, 0.1),
    color: theme.palette.primary.main,

    "&:hover": {
      background: alpha(theme.palette.primary.main, 0.3),
    },
  },
  confirmDialogCancelButton: (props: StyleProps) => {
    const color =
      props.type === "info"
        ? theme.palette.primary.contrastText
        : theme.palette.error.contrastText
    return {
      background: alpha(color, 0.15),
      color,

      "&:hover": {
        background: alpha(color, 0.3),
      },

      "&.Mui-disabled": {
        background: alpha(color, 0.15),
        color: alpha(color, 0.5),
      },
    }
  },
  submitButton: {
    // Override disabled to keep background color, change loading spinner to contrast color
    "&.Mui-disabled": {
      "&.MuiButton-containedPrimary": {
        background: theme.palette.primary.dark,

        "& .MuiCircularProgress-root": {
          color: theme.palette.primary.contrastText,
        },
      },

      "&.CdrButton-error.MuiButton-contained": {
        background: darken(theme.palette.error.main, 0.3),

        "& .MuiCircularProgress-root": {
          color: theme.palette.error.contrastText,
        },
      },
    },
  },
  errorButton: {
    "&.MuiButton-contained": {
      backgroundColor: lighten(theme.palette.error.dark, 0.15),
      color: theme.palette.error.contrastText,
      "&:hover": {
        backgroundColor: theme.palette.error.dark,
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
      color: theme.palette.error.main,
      borderColor: theme.palette.error.main,
      "&:hover": {
        backgroundColor: alpha(
          theme.palette.error.main,
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
      color: theme.palette.error.main,
      "&:hover": {
        backgroundColor: alpha(
          theme.palette.error.main,
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
