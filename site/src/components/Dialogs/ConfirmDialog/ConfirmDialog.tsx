import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import type { FC, ReactNode } from "react";
import { DialogActionButtons, type DialogActionButtonsProps } from "../Dialog";
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
			open={open}
			onOpenChange={(isOpen) => {
				if (!isOpen) onClose();
			}}
		>
			<DialogContent
				variant={type === "delete" ? "destructive" : "default"}
				className="max-w-[440px]"
				data-testid="dialog"
			>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
					{description && (
						<DialogDescription asChild>
							<div className="[&_strong]:text-content-primary [&_p]:my-2">
								{description}
							</div>
						</DialogDescription>
					)}
				</DialogHeader>
				<DialogFooter>
					<DialogActionButtons
						cancelText={cancelText}
						confirmLoading={confirmLoading}
						confirmText={confirmText || defaults.confirmText}
						disabled={disabled}
						onCancel={!hideCancel ? onClose : undefined}
						onConfirm={onConfirm || onClose}
						type={type}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
