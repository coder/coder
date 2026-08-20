import type { FC, ReactNode } from "react";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";

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
	readonly title: string;
	readonly open: boolean;
	readonly onClose: () => void;
	readonly description: ReactNode;
	readonly cancelText?: string;
	readonly confirmText?: ReactNode;
	readonly confirmLoading?: boolean;
	readonly disabled?: boolean;
	/**
	 * When omitted, `onClose` doubles as the confirm handler, so a `delete`
	 * dialog without `onConfirm` closes without deleting anything.
	 */
	readonly onConfirm?: () => void;
	readonly type?: ConfirmDialogType;
	/**
	 * Defaults to shown for "delete", hidden for "info"/"success".
	 */
	readonly hideCancel?: boolean;
	/**
	 * Forwarded to Radix. This dialog renders no `DialogTrigger`, so Radix has
	 * nothing to return focus to on close and it lands on `<body>`. Callers that
	 * open it from a control the user should return to can preventDefault here
	 * and focus that control instead.
	 */
	readonly onCloseAutoFocus?: (event: Event) => void;
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
	onCloseAutoFocus,
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
				onCloseAutoFocus={onCloseAutoFocus}
			>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
					<DialogDescription asChild>
						<div className="text-sm text-content-secondary font-medium [&_strong]:text-content-primary [&_p]:m-0 [&_p+p]:mt-2">
							{description}
						</div>
					</DialogDescription>
				</DialogHeader>

				<DialogFooter>
					<DialogActions
						cancelText={cancelText}
						onCancel={shouldHideCancel ? undefined : onClose}
						confirmText={resolvedConfirmText}
						confirmLoading={confirmLoading}
						confirmDisabled={disabled}
						confirmVariant={type === "delete" ? "destructive" : undefined}
						onConfirm={handleConfirm}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
