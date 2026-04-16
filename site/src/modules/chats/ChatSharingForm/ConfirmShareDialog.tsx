import { AlertTriangleIcon } from "lucide-react";
import { type FC, useEffect, useId, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";

export type ConfirmShareDialogProps = {
	open: boolean;
	isMutating: boolean;
	requiresToolCalls: boolean;
	requiresAttachments: boolean;
	message?: string;
	detail?: string;
	onConfirm: (confirm: {
		toolCalls?: boolean;
		attachments?: boolean;
	}) => void | Promise<void>;
	onCancel: () => void;
};

export const ConfirmShareDialog: FC<ConfirmShareDialogProps> = ({
	open,
	isMutating,
	requiresToolCalls,
	requiresAttachments,
	message,
	detail,
	onConfirm,
	onCancel,
}) => {
	const [acknowledgedToolCalls, setAcknowledgedToolCalls] = useState(false);
	const [acknowledgedAttachments, setAcknowledgedAttachments] = useState(false);
	const toolCallsId = useId();
	const attachmentsId = useId();

	// Reset the checkboxes each time the dialog opens so a previous
	// confirmation does not accidentally carry over.
	useEffect(() => {
		if (open) {
			setAcknowledgedToolCalls(false);
			setAcknowledgedAttachments(false);
		}
	}, [open]);

	const canConfirm =
		(!requiresToolCalls || acknowledgedToolCalls) &&
		(!requiresAttachments || acknowledgedAttachments);

	const handleConfirm = () => {
		if (!canConfirm) return;
		void onConfirm({
			toolCalls: requiresToolCalls ? true : undefined,
			attachments: requiresAttachments ? true : undefined,
		});
	};

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				if (!next) onCancel();
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						<AlertTriangleIcon
							className="size-icon-sm text-content-warning"
							aria-hidden="true"
						/>
						Confirm sharing
					</DialogTitle>
					<DialogDescription>
						{message ??
							"This chat contains content that shared viewers will be able to see."}
					</DialogDescription>
				</DialogHeader>

				<div className="flex flex-col gap-3 text-sm">
					{detail && <p className="text-content-secondary m-0">{detail}</p>}
					{requiresToolCalls && (
						<div className="flex items-start gap-2">
							<Checkbox
								id={toolCallsId}
								checked={acknowledgedToolCalls}
								onCheckedChange={(v) => setAcknowledgedToolCalls(v === true)}
								data-testid="confirm-share-tool-calls"
							/>
							<label htmlFor={toolCallsId} className="cursor-pointer">
								I understand shared viewers will see the tool calls already in
								this chat.
							</label>
						</div>
					)}
					{requiresAttachments && (
						<div className="flex items-start gap-2">
							<Checkbox
								id={attachmentsId}
								checked={acknowledgedAttachments}
								onCheckedChange={(v) => setAcknowledgedAttachments(v === true)}
								data-testid="confirm-share-attachments"
							/>
							<label htmlFor={attachmentsId} className="cursor-pointer">
								I understand shared viewers will see the attachments already in
								this chat.
							</label>
						</div>
					)}
				</div>

				<DialogFooter>
					<Button variant="outline" onClick={onCancel} disabled={isMutating}>
						Cancel
					</Button>
					<Button
						onClick={handleConfirm}
						disabled={!canConfirm || isMutating}
						data-testid="confirm-share-submit"
					>
						<Spinner loading={isMutating} />
						Share anyway
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
