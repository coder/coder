import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";

export type ConfirmDialogType = "delete" | "info" | "success";

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
	readonly title: string;
	readonly open: boolean;
	readonly onClose: () => void;
	readonly description?: ReactNode;
	readonly cancelText?: string;
	readonly confirmText?: ReactNode;
	readonly confirmLoading?: boolean;
	readonly disabled?: boolean;
	readonly onConfirm?: () => void;
	readonly type?: ConfirmDialogType;
	/**
	 * When undefined:
	 *   - cancel is hidden for "info" and "success" dialogs
	 *   - cancel is shown for "delete" dialogs
	 */
	readonly hideCancel?: boolean;
}

/**
 * Quick-use dialog for yes/no style confirmations without custom layout.
 */
export const ConfirmDialog: FC<ConfirmDialogProps> = ({
	cancelText = "Cancel",
	confirmLoading = false,
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
	const shouldHideCancel = hideCancel ?? defaults.hideCancel;
	const resolvedConfirmText = confirmText ?? defaults.confirmText;
	const handleConfirm = onConfirm ?? onClose;

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
				{...(!description ? { "aria-describedby": undefined } : {})}
			>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
					{description && (
						<DialogDescription asChild>
							<div className="text-sm text-content-secondary font-medium [&_strong]:text-content-primary [&_p]:m-0 [&_p+p]:mt-2">
								{description}
							</div>
						</DialogDescription>
					)}
				</DialogHeader>

				<DialogFooter>
					{!shouldHideCancel && (
						<Button
							type="button"
							variant="outline"
							disabled={confirmLoading}
							onClick={onClose}
						>
							{cancelText}
						</Button>
					)}
					<Button
						type="button"
						variant={type === "delete" ? "destructive" : undefined}
						disabled={confirmLoading || disabled}
						onClick={handleConfirm}
						data-testid="confirm-button"
					>
						<Spinner loading={confirmLoading} />
						{resolvedConfirmText}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
