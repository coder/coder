import DialogActions from "@mui/material/DialogActions";
import type { FC, ReactNode } from "react";
import { cn } from "utils/cn";
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
			className={cn(
				"[&_.MuiPaper-root]:bg-surface-secondary [&_.MuiPaper-root]:border",
				"[&_.MuiPaper-root]:border-solid [&_.MuiPaper-root]:w-full [&_.MuiPaper-root]:max-w-[440px]",
				"[&_.MuiDialogActions-spacing]:pt-0 [&_.MuiDialogActions-spacing]:px-10 [&_.MuiDialogActions-spacing]:pb-10",
			)}
			onClose={onClose}
			open={open}
			data-testid="dialog"
		>
			<div className="pt-10 px-10 pb-5 text-content-secondary">
				<h3 className="m-0 mb-4 font-normal text-xl text-content-primary">
					{title}
				</h3>
				{description && (
					<div className="text-content-secondary leading-relaxed text-base [&_strong]:text-content-primary [&_p:not(.MuiFormHelperText-root)]:m-0 [&_>p]:my-2">
						{description}
					</div>
				)}
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
