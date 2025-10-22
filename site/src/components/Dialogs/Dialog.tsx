import MuiDialog, { type DialogProps } from "@mui/material/Dialog";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import type { FC, ReactNode } from "react";
import type { ConfirmDialogType } from "./types";

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
				<Button
					disabled={confirmLoading}
					onClick={(e) => {
						e.stopPropagation();
						onCancel();
					}}
					variant="outline"
				>
					{cancelText}
				</Button>
			)}

			{onConfirm && (
				<Button
					variant={type === "delete" ? "destructive" : undefined}
					disabled={confirmLoading || disabled}
					onClick={onConfirm}
					data-testid="confirm-button"
					type="submit"
				>
					<Spinner loading={confirmLoading} />
					{confirmText}
				</Button>
			)}
		</>
	);
};

/**
 * Re-export of MUI's Dialog component, for convenience.
 * @link See original documentation here: https://mui.com/material-ui/react-dialog/
 */
export { MuiDialog as Dialog, type DialogProps };
