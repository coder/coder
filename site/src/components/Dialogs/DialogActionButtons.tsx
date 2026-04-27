import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import type { ConfirmDialogType } from "./types";

export interface DialogActionButtonsProps {
	/** Text to display in the cancel button. */
	cancelText?: string;
	/** Text to display in the confirm button. */
	confirmText?: ReactNode;
	/** Whether confirm is loading, also disables cancel when true. */
	confirmLoading?: boolean;
	/** Whether the submit button is disabled. */
	disabled?: boolean;
	/** Called when cancel is clicked. */
	onCancel?: () => void;
	/** Called when confirm is clicked. */
	onConfirm?: () => void;
	type?: ConfirmDialogType;
}

/**
 * Quickly handles modal actions with a cancel button, a confirm button, or both.
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
