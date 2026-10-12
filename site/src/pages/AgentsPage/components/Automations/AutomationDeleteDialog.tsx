import { useState } from "react";
import type { ChatAutomation } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

type AutomationDeleteDialogProps = {
	automation: ChatAutomation;
	/** False when an admin deletes another user's automation. */
	isOwner: boolean;
	isDeleting: boolean;
	error: unknown;
	onConfirm: () => void;
	onClose: () => void;
	/** Receives focus when a successful delete removed the opener. */
	fallbackFocusRef?: React.RefObject<HTMLElement | null>;
};

/** Confirms deleting an automation, from the list or its own page. */
export const AutomationDeleteDialog: React.FC<AutomationDeleteDialogProps> = ({
	automation,
	isOwner,
	isDeleting,
	error,
	onConfirm,
	onClose,
	fallbackFocusRef,
}) => {
	// This opens from a Delete button, so the focused element at mount is
	// the button to return to on cancel.
	const [opener] = useState(() => document.activeElement);
	const ownerName = automation.owner
		? automation.owner.name || automation.owner.username
		: "another user";
	return (
		<ConfirmDialog
			open
			type="delete"
			title={`Delete ${automation.name}?`}
			confirmLoading={isDeleting}
			onConfirm={onConfirm}
			// Closing mid-request would hand focus to a row that may vanish.
			onClose={() => {
				if (!isDeleting) {
					onClose();
				}
			}}
			onCloseAutoFocus={(event) => {
				const target =
					opener instanceof HTMLElement && opener.isConnected
						? opener
						: fallbackFocusRef?.current;
				if (target) {
					event.preventDefault();
					target.focus();
				}
			}}
			description={
				<>
					{!isOwner && (
						<p>
							This automation belongs to <strong>{ownerName}</strong>. Coder
							does not notify them, so tell them why you deleted it.
						</p>
					)}
					<p>
						{automation.kind === "schedule"
							? "Its schedule stops, and no more runs start."
							: "Its webhook endpoint stops accepting events."}{" "}
						Messages it queued that have not started yet are removed from their
						chats. Chats it created and messages it already sent stay, so its
						past runs stay in those chats.
					</p>
					<p>You cannot undo this.</p>
					{Boolean(error) && (
						<div className="mt-4">
							<ErrorAlert error={error} />
						</div>
					)}
				</>
			}
		/>
	);
};
