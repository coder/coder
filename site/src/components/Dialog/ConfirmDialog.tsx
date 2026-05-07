import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./Dialog";

type ConfirmDialogType = "delete" | "info" | "success";

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

export interface ConfirmDialogProps {
	readonly open: boolean;
	readonly title: string;
	readonly description?: ReactNode;
	readonly cancelText?: string;
	readonly confirmText?: ReactNode;
	readonly confirmLoading?: boolean;
	readonly disabled?: boolean;
	readonly type?: ConfirmDialogType;
	/**
	 * Hides the cancel button when set true, and shows the cancel button
	 * when set to false. When undefined:
	 *   - cancel is not displayed for "info" and "success" dialogs
	 *   - cancel is displayed for "delete" dialogs
	 */
	readonly hideCancel?: boolean;
	/**
	 * Called when canceling (if cancel is showing) and when the dialog
	 * is dismissed (e.g. via Escape or clicking the overlay).
	 *
	 * If onConfirm is not defined, onClose is used in its place when
	 * confirming.
	 */
	readonly onClose: () => void;
	readonly onConfirm?: () => void;
}

/**
 * Quick-use Dialog with slightly alternative styles, great for dialogs that
 * don't have any interaction beyond yes / no.
 */
export const ConfirmDialog: FC<ConfirmDialogProps> = ({
	open,
	title,
	description,
	cancelText = "Cancel",
	confirmText,
	confirmLoading = false,
	disabled = false,
	type = "info",
	hideCancel,
	onClose,
	onConfirm,
}) => {
	const defaults = CONFIRM_DIALOG_DEFAULTS[type];
	const showCancel = !(hideCancel ?? defaults.hideCancel);

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onClose();
				}
			}}
		>
			<DialogContent
				variant={type === "delete" ? "destructive" : "default"}
				data-testid="dialog"
				// Workaround for a Radix Dialog bug where opening the dialog from a
				// DropdownMenu item leaves `pointer-events: none` on the body after
				// close, blocking subsequent interaction. See:
				// https://github.com/radix-ui/primitives/issues/1241
				onCloseAutoFocus={() => {
					document.body.style.pointerEvents = "";
				}}
			>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
					{description && (
						<DialogDescription asChild>
							<div>{description}</div>
						</DialogDescription>
					)}
				</DialogHeader>

				<DialogFooter>
					{showCancel && (
						<Button
							variant="outline"
							disabled={confirmLoading}
							onClick={(e) => {
								e.stopPropagation();
								onClose();
							}}
						>
							{cancelText}
						</Button>
					)}
					<Button
						variant={type === "delete" ? "destructive" : "default"}
						disabled={confirmLoading || disabled}
						onClick={onConfirm ?? onClose}
						data-testid="confirm-button"
						type="submit"
					>
						<Spinner loading={confirmLoading} />
						{confirmText ?? defaults.confirmText}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
