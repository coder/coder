import MuiDialog, { DialogProps as MuiDialogProps } from "@mui/material/Dialog";
import LoadingButton, { LoadingButtonProps } from "@mui/lab/LoadingButton";
import { type Interpolation, type Theme } from "@emotion/react";
import { type FC, type ReactNode } from "react";
import { ConfirmDialogType } from "./types";

export interface DialogActionButtonsProps {
  /** Text to display in the cancel button */
  cancelText?: string;
  /** Text to display in the confirm button */
  confirmText?: ReactNode;
  /** Whether or not confirm is loading, also disables cancel when true */
  confirmLoading?: boolean;
  /** Whether or not the submit button is disabled */
  disabled?: boolean;
  /** Called when cancel is clicked */
  onCancel?: () => void;
  /** Called when confirm is clicked */
  onConfirm?: () => void;
  type?: ConfirmDialogType;
}

const typeToColor = (type: ConfirmDialogType): LoadingButtonProps["color"] => {
  if (type === "delete") {
    return "secondary";
  }
  return "primary";
};

/**
 * Quickly handles most modals actions, some combination of a cancel and confirm button
 */
export const DialogActionButtons: FC<DialogActionButtonsProps> = ({
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
          css={[
            type === "delete" && styles.dangerButton,
            type === "success" && styles.successButton,
          ]}
        >
          {confirmText}
        </LoadingButton>
      )}
    </>
  );
};

const styles = {
  dangerButton: (theme) => ({
    "&.MuiButton-contained": {
      backgroundColor: theme.experimental.roles.danger.fill,
      borderColor: theme.experimental.roles.danger.outline,

      "&:not(.MuiLoadingButton-loading)": {
        color: theme.experimental.roles.danger.text,
      },

      "&:hover:not(:disabled)": {
        backgroundColor: theme.experimental.roles.danger.disabled.fill,
        borderColor: theme.experimental.roles.danger.disabled.outline,
      },

      "&.Mui-disabled": {
        backgroundColor: theme.experimental.roles.danger.disabled.background,
        borderColor: theme.experimental.roles.danger.disabled.outline,

        "&:not(.MuiLoadingButton-loading)": {
          color: theme.experimental.roles.danger.disabled.text,
        },
      },
    },
  }),
  successButton: (theme) => ({
    "&.MuiButton-contained": {
      backgroundColor: theme.palette.success.dark,

      "&:not(.MuiLoadingButton-loading)": {
        color: theme.palette.primary.contrastText,
      },

      "&:hover": {
        backgroundColor: theme.palette.success.main,

        "@media (hover: none)": {
          backgroundColor: "transparent",
        },

        "&.Mui-disabled": {
          backgroundColor: "transparent",
        },
      },

      "&.Mui-disabled": {
        backgroundColor: theme.palette.success.dark,

        "&:not(.MuiLoadingButton-loading)": {
          color: theme.palette.text.secondary,
        },
      },
    },

    "&.MuiButton-outlined": {
      color: theme.palette.success.main,
      borderColor: theme.palette.success.main,
      "&:hover": {
        backgroundColor: theme.palette.success.dark,
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
        "&.Mui-disabled": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: theme.palette.text.secondary,
        borderColor: theme.palette.action.disabled,
      },
    },

    "&.MuiButton-text": {
      color: theme.palette.success.main,
      "&:hover": {
        backgroundColor: theme.palette.success.dark,
        "@media (hover: none)": {
          backgroundColor: "transparent",
        },
      },
      "&.Mui-disabled": {
        color: theme.palette.text.secondary,
      },
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;

export type DialogProps = MuiDialogProps;

/**
 * Re-export of MUI's Dialog component, for convenience.
 * @link See original documentation here: https://mui.com/material-ui/react-dialog/
 */
export { MuiDialog as Dialog };
