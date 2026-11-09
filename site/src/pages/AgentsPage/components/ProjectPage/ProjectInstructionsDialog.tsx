import { useId, useRef, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { getErrorMessage, isApiError } from "#/api/errors";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { InvisibleCharactersAlert } from "#/components/Alert/InvisibleCharactersAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { sanitizePromptText } from "#/utils/invisibleUnicode";

type ProjectInstructionsDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	/** The saved instructions, or "" when the project has none. */
	readonly instructions: string;
	readonly isSaving: boolean;
	readonly isDeleting: boolean;
	readonly saveError: unknown;
	readonly deleteError: unknown;
	/**
	 * Called whenever the draft changes and before a save or delete starts,
	 * so a stale error from the other action is never shown.
	 */
	readonly onResetErrors: () => void;
	readonly onSave: (instructions: string) => void;
	readonly onDelete: () => void;
};

export const ProjectInstructionsDialog: React.FC<
	ProjectInstructionsDialogProps
> = ({ open, onOpenChange, ...formProps }) => {
	const isBusy = formProps.isSaving || formProps.isDeleting;
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !isBusy) {
			onOpenChange(false);
		}
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent>
				<ProjectInstructionsForm
					{...formProps}
					open={open}
					onCancel={() => handleOpenChange(false)}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ProjectInstructionsFormProps = Omit<
	ProjectInstructionsDialogProps,
	"onOpenChange"
> & {
	readonly onCancel: () => void;
};

const ProjectInstructionsForm: React.FC<ProjectInstructionsFormProps> = ({
	open,
	instructions,
	isSaving,
	isDeleting,
	saveError,
	deleteError,
	onResetErrors,
	onSave,
	onDelete,
	onCancel,
}) => {
	const textareaId = useId();
	const deleteButtonRef = useRef<HTMLButtonElement>(null);
	const loadLatestButtonRef = useRef<HTMLButtonElement>(null);
	const [baseInstructions, setBaseInstructions] = useState(instructions);
	const [draft, setDraft] = useState(baseInstructions);
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const hasConflict = open && instructions !== baseInstructions;
	// The confirmation would hide the conflict warning behind its overlay.
	if (hasConflict && isConfirmingDelete) {
		setIsConfirmingDelete(false);
	}
	const isEditing = baseInstructions !== "";
	const isBusy = isSaving || isDeleting;
	const error = saveError ?? deleteError;
	const errorFallback = saveError
		? "Failed to save instructions."
		: "Failed to delete instructions.";
	const errorDetail = isApiError(error)
		? error.response.data.detail
		: undefined;
	const normalizedDraft = sanitizePromptText(draft);
	const canSave =
		normalizedDraft !== "" &&
		normalizedDraft !== sanitizePromptText(baseInstructions) &&
		!hasConflict &&
		!isBusy;

	return (
		<>
			<form
				className="flex flex-col gap-4"
				onSubmit={(event) => {
					event.preventDefault();
					if (canSave) {
						onResetErrors();
						onSave(draft);
					}
				}}
			>
				<DialogHeader>
					<DialogTitle>
						{isEditing ? "Edit instructions" : "Create instructions"}
					</DialogTitle>
					<DialogDescription>
						Instructions are added to every chat in this project, for all users.
					</DialogDescription>
				</DialogHeader>
				<label htmlFor={textareaId} className="sr-only">
					Instructions
				</label>
				<TextareaAutosize
					id={textareaId}
					className="max-h-[50vh] w-full resize-none overflow-y-auto rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-hidden focus:ring-2 focus:ring-content-link scrollbar-thin"
					placeholder="Conventions, context, and preferences for every chat in this project"
					value={draft}
					onChange={(event) => {
						setDraft(event.target.value);
						onResetErrors();
					}}
					disabled={isBusy}
					minRows={6}
					autoFocus
				/>
				{hasConflict && (
					<Alert
						severity="warning"
						actions={
							<div className="flex gap-2">
								<Button
									type="button"
									size="sm"
									variant="outline"
									disabled={isBusy}
									onClick={() => setBaseInstructions(instructions)}
								>
									Keep my draft
								</Button>
								<Button
									ref={loadLatestButtonRef}
									type="button"
									size="sm"
									disabled={isBusy}
									onClick={() => {
										setBaseInstructions(instructions);
										setDraft(instructions);
										onResetErrors();
									}}
								>
									Load latest
								</Button>
							</div>
						}
					>
						<AlertDescription>
							Someone else changed these instructions while you were editing.
							Load the latest version, or keep your draft to replace it.
						</AlertDescription>
					</Alert>
				)}
				<InvisibleCharactersAlert text={draft} />
				{Boolean(error) && (
					<Alert severity="error" prominent>
						<AlertTitle>{getErrorMessage(error, errorFallback)}</AlertTitle>
						{errorDetail && <AlertDescription>{errorDetail}</AlertDescription>}
					</Alert>
				)}
				<DialogFooter>
					{isEditing && (
						<Button
							ref={deleteButtonRef}
							type="button"
							variant="destructive"
							className="sm:mr-auto"
							disabled={isBusy || hasConflict}
							onClick={() => setIsConfirmingDelete(true)}
						>
							<Spinner loading={isDeleting} />
							Delete
						</Button>
					)}
					<Button
						type="button"
						variant="outline"
						disabled={isBusy}
						onClick={onCancel}
					>
						Cancel
					</Button>
					<Button type="submit" disabled={!canSave}>
						<Spinner loading={isSaving} />
						Save
					</Button>
				</DialogFooter>
			</form>
			<ConfirmDialog
				open={isConfirmingDelete}
				type="delete"
				title="Delete instructions?"
				description="Chats in this project will no longer receive these instructions, for all users. This action cannot be undone."
				confirmText="Delete instructions"
				onClose={() => setIsConfirmingDelete(false)}
				onConfirm={() => {
					setIsConfirmingDelete(false);
					onResetErrors();
					onDelete();
				}}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					// A conflict closes the confirmation and disables Delete, so
					// focus the conflict's primary action instead.
					const focusTarget = hasConflict
						? loadLatestButtonRef.current
						: deleteButtonRef.current;
					focusTarget?.focus();
				}}
			/>
		</>
	);
};
