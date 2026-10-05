import { useId, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
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
import {
	countInvisibleCharacters,
	removeInvisibleCharacters,
} from "#/utils/invisibleUnicode";

type ProjectInstructionsDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	/** The saved instructions, or "" when the project has none. */
	readonly instructions: string;
	readonly isSaving: boolean;
	readonly isDeleting: boolean;
	readonly error: unknown;
	/** Called when the user edits the draft, so stale errors can clear. */
	readonly onDraftChange: () => void;
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
			{/* Radix unmounts the content on close, so the draft below resets
			    to the saved instructions on every open. */}
			<DialogContent>
				<ProjectInstructionsForm
					{...formProps}
					onCancel={() => handleOpenChange(false)}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ProjectInstructionsFormProps = Omit<
	ProjectInstructionsDialogProps,
	"open" | "onOpenChange"
> & {
	readonly onCancel: () => void;
};

const ProjectInstructionsForm: React.FC<ProjectInstructionsFormProps> = ({
	instructions,
	isSaving,
	isDeleting,
	error,
	onDraftChange,
	onSave,
	onDelete,
	onCancel,
}) => {
	const textareaId = useId();
	const [draft, setDraft] = useState(instructions);
	const isEditing = instructions !== "";
	const isBusy = isSaving || isDeleting;
	const invisibleCharCount = countInvisibleCharacters(draft);
	const errorDetail = getErrorDetail(error);
	// Blank instructions are cleared with Delete rather than saved, and
	// whitespace-only edits at either end are not a change worth saving.
	// The server strips invisible characters, so text made only of them
	// counts as blank.
	const canSave =
		removeInvisibleCharacters(draft).trim() !== "" &&
		draft.trim() !== instructions.trim() &&
		!isBusy;

	return (
		<form
			className="flex flex-col gap-4"
			onSubmit={(event) => {
				event.preventDefault();
				if (canSave) {
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
				className="max-h-[50vh] w-full resize-none overflow-y-auto rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-hidden focus:ring-2 focus:ring-content-link"
				placeholder="Conventions, context, and preferences for every chat in this project"
				value={draft}
				onChange={(event) => {
					setDraft(event.target.value);
					onDraftChange();
				}}
				disabled={isBusy}
				minRows={6}
				autoFocus
			/>
			{invisibleCharCount > 0 && (
				<Alert severity="warning">
					<AlertDescription>
						This text contains {invisibleCharCount} invisible Unicode{" "}
						{invisibleCharCount !== 1 ? "characters" : "character"} that could
						hide content. These will be stripped on save.
					</AlertDescription>
				</Alert>
			)}
			{Boolean(error) && (
				<div className="text-sm text-content-destructive">
					<p className="m-0">
						{getErrorMessage(error, "Failed to save instructions.")}
					</p>
					{errorDetail && <p className="m-0 mt-1">{errorDetail}</p>}
				</div>
			)}
			<DialogFooter>
				{isEditing && (
					<Button
						type="button"
						variant="destructive"
						className="sm:mr-auto"
						disabled={isBusy}
						onClick={onDelete}
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
	);
};
