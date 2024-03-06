import type { Interpolation, Theme } from "@emotion/react";
import DialogActions from "@mui/material/DialogActions";
import type { FC, ReactNode } from "react";
import {
  Dialog,
  DialogActionButtons,
  type DialogActionButtonsProps,
} from "../Dialog";
import type { ConfirmDialogType } from "../types";

interface ConfirmDialogTypeConfig {
  confirmText: ReactNode;
  hideCancel: boolean;
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
};

export interface ConfirmDialogProps
  extends Omit<DialogActionButtonsProps, "color" | "onCancel"> {
  readonly description?: ReactNode;
  /**
   * hideCancel hides the cancel button when set true, and shows the cancel
   * button when set to false. When undefined:
   *   - cancel is not displayed for "info" dialogs
   *   - cancel is displayed for "delete" dialogs
   */
  readonly hideCancel?: boolean;
  /**
   * onClose is called when canceling (if cancel is showing).
   *
   * Additionally, if onConfirm is not defined onClose will be used in its place
   * when confirming.
   */
  readonly onClose: () => void;
  readonly open: boolean;
  readonly title: string;
}

const styles = {
  dialogWrapper: (theme) => ({
    "& .MuiPaper-root": {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      width: "100%",
      maxWidth: 440,
    },
    "& .MuiDialogActions-spacing": {
      padding: "0 40px 40px",
    },
  }),
  dialogContent: (theme) => ({
    color: theme.palette.text.secondary,
    padding: "40px 40px 20px",
  }),
  dialogTitle: (theme) => ({
    margin: 0,
    marginBottom: 16,
    color: theme.palette.text.primary,
    fontWeight: 400,
    fontSize: 20,
  }),
  dialogDescription: (theme) => ({
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    fontSize: 16,

    "& strong": {
      color: theme.palette.text.primary,
    },

    "& p:not(.MuiFormHelperText-root)": {
      margin: 0,
    },

    "& > p": {
      margin: "8px 0",
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;

/**
 * Quick-use version of the Dialog component with slightly alternative styles,
 * great to use for dialogs that don't have any interaction beyond yes / no.
 */
export const ConfirmDialog: FC<ConfirmDialogProps> = ({
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
  const defaults = CONFIRM_DIALOG_DEFAULTS[type];

  if (typeof hideCancel === "undefined") {
    hideCancel = defaults.hideCancel;
  }

  return (
    <Dialog
      css={styles.dialogWrapper}
      onClose={onClose}
      open={open}
      data-testid="dialog"
    >
      <div css={styles.dialogContent}>
        <h3 css={styles.dialogTitle}>{title}</h3>
        {description && <div css={styles.dialogDescription}>{description}</div>}
      </div>

      <DialogActions>
        <DialogActionButtons
          cancelText={cancelText}
          confirmLoading={confirmLoading}
          confirmText={confirmText || defaults.confirmText}
          disabled={disabled}
          onCancel={!hideCancel ? onClose : undefined}
          onConfirm={onConfirm || onClose}
          type={type}
        />
      </DialogActions>
    </Dialog>
  );
};
