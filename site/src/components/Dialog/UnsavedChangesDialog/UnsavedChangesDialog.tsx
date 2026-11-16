import { TriangleAlertIcon } from "lucide-react";
import { useRef } from "react";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

type UnsavedChangesDialogProps = {
	open: boolean;
	onClose: () => void;
	onConfirm: () => void;
};

/**
 * Confirms leaving without saving; callers own dirty-state and navigation decisions.
 */
export const UnsavedChangesDialog: React.FC<UnsavedChangesDialogProps> = ({
	open,
	onClose,
	onConfirm,
}) => {
	const previousFocusRef = useRef<HTMLElement | null>(null);

	return (
		<ConfirmDialog
			type="info"
			hideCancel={false}
			open={open}
			onClose={onClose}
			onConfirm={onConfirm}
			onOpenAutoFocus={() => {
				previousFocusRef.current =
					document.activeElement instanceof HTMLElement
						? document.activeElement
						: null;
			}}
			onCloseAutoFocus={(event) => {
				event.preventDefault();
				if (
					document.activeElement === document.body &&
					previousFocusRef.current?.isConnected
				) {
					previousFocusRef.current.focus({ preventScroll: true });
				}
			}}
			title="Unsaved changes"
			cancelText="Stay"
			confirmText="Leave"
			description={
				<div className="flex items-start gap-3">
					<TriangleAlertIcon
						aria-hidden="true"
						className="size-icon-sm mt-1 shrink-0"
					/>
					<p className="m-0">Your updates haven't been saved. Leave anyway?</p>
				</div>
			}
		/>
	);
};
