import MuiDialog, { DialogProps as MuiDialogProps } from "@mui/material/Dialog";
import { type ReactNode } from "react";
import { colors } from "theme/colors";
import {
  LoadingButton,
  LoadingButtonProps,
} from "../LoadingButton/LoadingButton";
import { ConfirmDialogType } from "./types";
import { type Interpolation, type Theme } from "@emotion/react";
import { dark } from "theme/theme";

export interface DialogActionButtonsProps {
  /** Text to display in the cancel button */
  cancelText?: string;
  /** Text to display in the confirm button */
  confirmText?: ReactNode;
  /** Whether or not confirm is loading, also disables cancel when true */
  confirmLoading?: boolean;
  /** Whether or not this is a confirm dialog */
  confirmDialog?: boolean;
  /** Whether or not the submit button is disabled */
  disabled?: boolean;
  /** Called when cancel is clicked */
  onCancel?: () => void;
  /** Called when confirm is clicked */
  onConfirm?: () => void;
  type?: ConfirmDialogType;
}

const typeToColor = (type: ConfirmDialogType): LoadingButtonProps["color"] => {
  if (type === "danger") {
    return "secondary";
  }
  return "primary";
};

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
          css={[
            type === "danger" && styles.dangerButton,
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
      backgroundColor: dark.roles.danger.fill,
      borderColor: dark.roles.danger.outline,
      color: dark.roles.danger.text,

      "&:hover:not(:disabled)": {
        backgroundColor: dark.roles.danger.hover.fill,
        borderColor: dark.roles.danger.hover.outline,
        color: dark.roles.danger.hover.text,
      },

      "&.Mui-disabled": {
        backgroundColor: dark.roles.danger.disabled.fill,
        borderColor: dark.roles.danger.disabled.outline,
        color: dark.roles.danger.disabled.text,
      },
    },
  }),
  successButton: (theme) => ({
    "&.MuiButton-contained": {
      backgroundColor: dark.roles.success.fill,
      borderColor: dark.roles.success.outline,
      color: dark.roles.success.text,

      "&:hover:not(:disabled)": {
        backgroundColor: dark.roles.success.hover.fill,
        borderColor: dark.roles.success.hover.outline,
        color: dark.roles.success.hover.text,
      },

      "&.Mui-disabled": {
        backgroundColor: dark.roles.success.disabled.fill,
        borderColor: dark.roles.success.disabled.outline,
        color: dark.roles.success.disabled.text,
      },
    },

    // I wanna use the version about instead. bit cleaner, and matches the danger mode.
    // "&.MuiButton-contained": {
    //   backgroundColor: theme.palette.success.main,
    //   color: theme.palette.primary.contrastText,
    //   "&:hover": {
    //     backgroundColor: theme.palette.success.dark,
    //     "@media (hover: none)": {
    //       backgroundColor: "transparent",
    //     },
    //     "&.Mui-disabled": {
    //       backgroundColor: "transparent",
    //     },
    //   },
    //   "&.Mui-disabled": {
    //     backgroundColor: theme.palette.action.disabledBackground,
    //     color: theme.palette.text.secondary,
    //   },
    // },

    // TODO: do we need this?
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

    // TODO: do we need this?
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
 * Wrapper around Material UI's Dialog component. Conveniently exports all of
 * Dialog's components in one import, so for example `<DialogContent />` becomes
 * `<Dialog.Content />` etc. Also contains some custom Dialog components listed below.
 *
 * See original component's Material UI documentation here: https://material-ui.com/components/dialogs/
 */
export const Dialog: React.FC<DialogProps> = (props) => {
  // Wrapped so we can add custom attributes below
  return <MuiDialog {...props} />;
};
