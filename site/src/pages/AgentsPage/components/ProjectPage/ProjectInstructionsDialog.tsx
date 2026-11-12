import { useId, useRef, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { getErrorMessage, isApiError } from "#/api/errors";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { InvisibleCharactersAlert } from "#/components/Alert/InvisibleCharactersAlert";
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
	const textareaRef = useRef<HTMLTextAreaElement>(null);
	const [baseInstructions, setBaseInstructions] = useState(instructions);
	const [draft, setDraft] = useState(baseInstructions);
	const hasConflict = open && instructions !== baseInstructions;
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
	// Saving an empty draft removes the saved instructions. Clear only empties
	// the draft, so Cancel still discards the removal.
	const isRemoving = isEditing && normalizedDraft === "";
	const canSave =
		(normalizedDraft !== "" || isRemoving) &&
		normalizedDraft !== sanitizePromptText(baseInstructions) &&
		!hasConflict &&
		!isBusy;
	const changeDraft = (nextDraft: string) => {
		setDraft(nextDraft);
		onResetErrors();
	};

	return (
		<form
			className="flex flex-col gap-4"
			onSubmit={(event) => {
				event.preventDefault();
				if (!canSave) {
					return;
				}
				onResetErrors();
				if (isRemoving) {
					onDelete();
				} else {
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
				ref={textareaRef}
				id={textareaId}
				className="max-h-[50vh] w-full resize-none overflow-y-auto rounded-lg border bg-surface-primary px-4 py-3 font-sans text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-hidden focus:ring-2 focus:ring-content-link scrollbar-thin"
				placeholder="Conventions, context, and preferences for every chat in this project"
				value={draft}
				onChange={(event) => changeDraft(event.target.value)}
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
						Someone else changed these instructions while you were editing. Load
						the latest version, or keep your draft to replace it.
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
				<Button
					type="button"
					variant="subtle"
					className="sm:mr-auto"
					disabled={isBusy || draft === ""}
					onClick={() => {
						changeDraft("");
						textareaRef.current?.focus();
					}}
				>
					Clear
				</Button>
				<Button
					type="button"
					variant="outline"
					disabled={isBusy}
					onClick={onCancel}
				>
					Cancel
				</Button>
				<Button type="submit" disabled={!canSave}>
					<Spinner loading={isBusy} />
					Save
				</Button>
			</DialogFooter>
		</form>
	);
};
