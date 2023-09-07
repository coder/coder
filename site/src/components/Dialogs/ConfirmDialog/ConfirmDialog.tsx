import DialogActions from "@mui/material/DialogActions";
import { makeStyles } from "@mui/styles";
import { ReactNode, FC, PropsWithChildren } from "react";
import {
  Dialog,
  DialogActionButtons,
  DialogActionButtonsProps,
} from "../Dialog";
import { ConfirmDialogType } from "../types";
import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import { Stack } from "@mui/system";

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
  extends Omit<
    DialogActionButtonsProps,
    "color" | "confirmDialog" | "onCancel"
  > {
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

const useStyles = makeStyles((theme) => ({
  dialogWrapper: {
    "& .MuiPaper-root": {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      width: "100%",
      maxWidth: theme.spacing(55),
    },
    "& .MuiDialogActions-spacing": {
      padding: `0 ${theme.spacing(5)} ${theme.spacing(5)}`,
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

    "& p:not(.MuiFormHelperText-root)": {
      margin: 0,
    },

    "& > p": {
      margin: theme.spacing(1, 0),
    },
  },
}));

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
  const styles = useStyles({ type });

  const defaults = CONFIRM_DIALOG_DEFAULTS[type];

  if (typeof hideCancel === "undefined") {
    hideCancel = defaults.hideCancel;
  }

  return (
    <Dialog
      className={styles.dialogWrapper}
      onClose={onClose}
      open={open}
      data-testid="dialog"
    >
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
  );
};

export interface ScheduleDialogProps extends ConfirmDialogProps {
  readonly inactiveWorkspacesToGoDormant: number;
  readonly inactiveWorkspacesToGoDormantInWeek: number;
  readonly dormantWorkspacesToBeDeleted: number;
  readonly dormantWorkspacesToBeDeletedInWeek: number;
  readonly updateDormantWorkspaces: (confirm: boolean) => void;
  readonly updateInactiveWorkspaces: (confirm: boolean) => void;
  readonly dormantValueChanged: boolean;
  readonly deletionValueChanged: boolean;
}

export const ScheduleDialog: FC<PropsWithChildren<ScheduleDialogProps>> = ({
  cancelText,
  confirmLoading,
  disabled = false,
  hideCancel,
  onClose,
  onConfirm,
  type,
  open = false,
  title,
  inactiveWorkspacesToGoDormant,
  inactiveWorkspacesToGoDormantInWeek,
  dormantWorkspacesToBeDeleted,
  dormantWorkspacesToBeDeletedInWeek,
  updateDormantWorkspaces,
  updateInactiveWorkspaces,
  dormantValueChanged,
  deletionValueChanged,
}) => {
  const styles = useScheduleStyles({ type });

  const defaults = CONFIRM_DIALOG_DEFAULTS["delete"];

  if (typeof hideCancel === "undefined") {
    hideCancel = defaults.hideCancel;
  }

  const showDormancyWarning =
    dormantValueChanged &&
    (inactiveWorkspacesToGoDormant > 0 ||
      inactiveWorkspacesToGoDormantInWeek > 0);
  const showDeletionWarning =
    deletionValueChanged &&
    (dormantWorkspacesToBeDeleted > 0 ||
      dormantWorkspacesToBeDeletedInWeek > 0);

  return (
    <Dialog
      className={styles.dialogWrapper}
      onClose={onClose}
      open={open}
      data-testid="dialog"
    >
      <div className={styles.dialogContent}>
        <h3 className={styles.dialogTitle}>{title}</h3>
        <>
          {showDormancyWarning && (
            <>
              <h4>{"Dormancy Threshold"}</h4>
              <Stack direction="row" spacing={5}>
                <div className={styles.dialogDescription}>{`
                This change will result in ${inactiveWorkspacesToGoDormant} workspaces being immediately transitioned to the dormant state and ${inactiveWorkspacesToGoDormantInWeek} over the next seven days. To prevent this, do you want to reset the inactivity period for all template workspaces?`}</div>
                <FormControlLabel
                  sx={{
                    marginTop: 2,
                  }}
                  control={
                    <Checkbox
                      size="small"
                      onChange={(e) => {
                        updateInactiveWorkspaces(e.target.checked);
                      }}
                    />
                  }
                  label="Reset"
                />
              </Stack>
            </>
          )}

          {showDeletionWarning && (
            <>
              <h4>{"Dormancy Auto-Deletion"}</h4>
              <Stack direction="row" spacing={5}>
                <div
                  className={styles.dialogDescription}
                >{`This change will result in ${dormantWorkspacesToBeDeleted} workspaces being immediately deleted and ${dormantWorkspacesToBeDeletedInWeek} over the next 7 days. To prevent this, do you want to reset the dormancy period for all template workspaces?`}</div>
                <FormControlLabel
                  sx={{
                    marginTop: 2,
                  }}
                  control={
                    <Checkbox
                      size="small"
                      onChange={(e) => {
                        updateDormantWorkspaces(e.target.checked);
                      }}
                    />
                  }
                  label="Reset"
                />
              </Stack>
            </>
          )}
        </>
      </div>

      <DialogActions>
        <DialogActionButtons
          cancelText={cancelText}
          confirmDialog
          confirmLoading={confirmLoading}
          confirmText="Submit"
          disabled={disabled}
          onCancel={!hideCancel ? onClose : undefined}
          onConfirm={onConfirm || onClose}
          type="delete"
        />
      </DialogActions>
    </Dialog>
  );
};

const useScheduleStyles = makeStyles((theme) => ({
  dialogWrapper: {
    "& .MuiPaper-root": {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      width: "100%",
      maxWidth: theme.spacing(125),
    },
    "& .MuiDialogActions-spacing": {
      padding: `0 ${theme.spacing(5)} ${theme.spacing(5)}`,
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

    "& p:not(.MuiFormHelperText-root)": {
      margin: 0,
    },

    "& > p": {
      margin: theme.spacing(1, 0),
    },
  },
}));
