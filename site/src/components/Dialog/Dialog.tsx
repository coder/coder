import MuiDialog, { DialogProps as MuiDialogProps } from "@material-ui/core/Dialog"
import MuiDialogTitle from "@material-ui/core/DialogTitle"
import InputAdornment from "@material-ui/core/InputAdornment"
import OutlinedInput, { OutlinedInputProps } from "@material-ui/core/OutlinedInput"
import { darken, fade, lighten, makeStyles } from "@material-ui/core/styles"
import SvgIcon from "@material-ui/core/SvgIcon"
import * as React from "react"
import { combineClasses } from "../../util/combineClasses"
import { SearchIcon } from "../Icons/SearchIcon"
import { LoadingButton, LoadingButtonProps } from "../LoadingButton/LoadingButton"
import { ConfirmDialogType } from "./types"

export interface DialogTitleProps {
  /** Title for display */
  title: React.ReactNode
  /** Optional icon to display faded to the right of the title */
  icon?: typeof SvgIcon
  /** Smaller text to display above the title */
  superTitle?: React.ReactNode
}

/**
 * Override of Material UI's DialogTitle that allows for a supertitle and background icon
 */
export const DialogTitle: React.FC<React.PropsWithChildren<DialogTitleProps>> = ({ title, icon: Icon, superTitle }) => {
  const styles = useTitleStyles()
  return (
    <MuiDialogTitle disableTypography>
      <div className={styles.titleWrapper}>
        {superTitle && <div className={styles.superTitle}>{superTitle}</div>}
        <div className={styles.title}>{title}</div>
      </div>
      {Icon && <Icon className={styles.icon} />}
    </MuiDialogTitle>
  )
}

const useTitleStyles = makeStyles(
  (theme) => ({
    title: {
      position: "relative",
      zIndex: 2,
      fontSize: theme.typography.h3.fontSize,
      fontWeight: theme.typography.h3.fontWeight,
      lineHeight: "40px",
      display: "flex",
      alignItems: "center",
    },
    superTitle: {
      position: "relative",
      zIndex: 2,
      fontSize: theme.typography.body2.fontSize,
      fontWeight: 500,
      letterSpacing: 1.5,
      textTransform: "uppercase",
    },
    titleWrapper: {
      padding: `${theme.spacing(2)}px 0`,
    },
    icon: {
      height: 84,
      width: 84,
      color: fade(theme.palette.action.disabled, 0.4),
    },
  }),
  { name: "CdrDialogTitle" },
)

export interface DialogActionButtonsProps {
  /** Text to display in the cancel button */
  cancelText?: string
  /** Text to display in the confirm button */
  confirmText?: React.ReactNode
  /** Whether or not confirm is loading, also disables cancel when true */
  confirmLoading?: boolean
  /** Whether or not this is a confirm dialog */
  confirmDialog?: boolean
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
export const DialogActionButtons: React.FC<React.PropsWithChildren<DialogActionButtonsProps>> = ({
  cancelText = "Cancel",
  confirmText = "Confirm",
  confirmLoading = false,
  confirmDialog,
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
    background: fade(theme.palette.primary.main, 0.1),
    color: theme.palette.primary.main,

    "&:hover": {
      background: fade(theme.palette.primary.main, 0.3),
    },
  },
  confirmDialogCancelButton: (props: StyleProps) => {
    const color =
      props.type === "info" ? theme.palette.primary.contrastText : theme.palette.error.contrastText
    return {
      background: fade(color, 0.15),
      color,

      "&:hover": {
        background: fade(color, 0.3),
      },

      "&.Mui-disabled": {
        background: fade(color, 0.15),
        color: fade(color, 0.5),
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
        color: fade(theme.palette.text.disabled, 0.5),
      },
    },

    "&.MuiButton-outlined": {
      color: theme.palette.error.main,
      borderColor: theme.palette.error.main,
      "&:hover": {
        backgroundColor: fade(theme.palette.error.main, theme.palette.action.hoverOpacity),
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
        "&.Mui-disabled": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: fade(theme.palette.text.disabled, 0.5),
        borderColor: theme.palette.action.disabled,
      },
    },

    "&.MuiButton-text": {
      color: theme.palette.error.main,
      "&:hover": {
        backgroundColor: fade(theme.palette.error.main, theme.palette.action.hoverOpacity),
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: fade(theme.palette.text.disabled, 0.5),
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
        color: fade(theme.palette.text.disabled, 0.5),
      },
    },

    "&.MuiButton-outlined": {
      color: theme.palette.success.main,
      borderColor: theme.palette.success.main,
      "&:hover": {
        backgroundColor: fade(theme.palette.success.main, theme.palette.action.hoverOpacity),
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
        "&.Mui-disabled": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: fade(theme.palette.text.disabled, 0.5),
        borderColor: theme.palette.action.disabled,
      },
    },

    "&.MuiButton-text": {
      color: theme.palette.success.main,
      "&:hover": {
        backgroundColor: fade(theme.palette.success.main, theme.palette.action.hoverOpacity),
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: fade(theme.palette.text.disabled, 0.5),
      },
    },
  },
}))

export type DialogSearchProps = Omit<
  OutlinedInputProps,
  "className" | "fullWidth" | "labelWidth" | "startAdornment"
>

/**
 * Formats a search bar right below the title of a Dialog. Passes all props
 * through to the Material UI OutlinedInput component contained within.
 */
export const DialogSearch: React.FC<React.PropsWithChildren<DialogSearchProps>> = (props) => {
  const styles = useSearchStyles()
  return (
    <div className={styles.root}>
      <OutlinedInput
        {...props}
        fullWidth
        labelWidth={0}
        className={styles.input}
        startAdornment={
          <InputAdornment position="start">
            <SearchIcon className={styles.icon} />
          </InputAdornment>
        }
      />
    </div>
  )
}

const useSearchStyles = makeStyles(
  (theme) => ({
    root: {
      position: "relative",
      padding: `${theme.spacing(2)}px ${theme.spacing(4)}px`,
      boxShadow: `0 2px 6px ${fade("#1D407E", 0.2)}`,
      zIndex: 2,
    },
    input: {
      margin: 0,
    },
    icon: {
      width: 16,
      height: 16,
    },
  }),
  { name: "CdrDialogSearch" },
)

export type DialogProps = MuiDialogProps

/**
 * Wrapper around Material UI's Dialog component. Conveniently exports all of
 * Dialog's components in one import, so for example `<DialogContent />` becomes
 * `<Dialog.Content />` etc. Also contains some custom Dialog components listed below.
 *
 * See original component's Material UI documentation here: https://material-ui.com/components/dialogs/
 */
export const Dialog: React.FC<React.PropsWithChildren<DialogProps>> = (props) => {
  // Wrapped so we can add custom attributes below
  return <MuiDialog {...props} />
}
