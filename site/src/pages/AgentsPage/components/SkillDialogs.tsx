import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
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
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import { SkillEditor } from "./SkillEditor";
import type {
	SkillDeleteState,
	SkillEditorState,
	SkillsCopy,
} from "./SkillsTableView";

type DialogFocusProps = {
	onCloseAutoFocus: (event: Event) => void;
};

export const EditSkillDialog: React.FC<
	DialogFocusProps & {
		copy: SkillsCopy;
		state: Extract<SkillEditorState, { mode: "edit" }>;
	}
> = ({ copy, state, onCloseAutoFocus }) => {
	const lowerNoun = copy.noun.toLocaleLowerCase("en-US");
	const handleOpenChange = (open: boolean) => {
		if (!open) {
			state.onClose();
		}
	};

	if (state.isLoading) {
		return (
			<Dialog open onOpenChange={handleOpenChange}>
				<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
					<DialogHeader>
						<DialogTitle>Loading {lowerNoun}</DialogTitle>
						<DialogDescription>
							Fetching the latest SKILL.md content.
						</DialogDescription>
					</DialogHeader>
					<Loader />
				</DialogContent>
			</Dialog>
		);
	}

	if (state.loadError || !state.initialValues) {
		return (
			<Dialog open onOpenChange={handleOpenChange}>
				<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
					<DialogHeader>
						<DialogTitle>Unable to load {lowerNoun}</DialogTitle>
						<DialogDescription>
							The skill could not be loaded for{" "}
							{state.readOnly ? "viewing" : "editing"}.
						</DialogDescription>
					</DialogHeader>
					{state.loadError ? (
						<ErrorAlert error={state.loadError} showDebugDetail={false} />
					) : (
						<Alert severity="error">
							<AlertDescription>
								The saved content could not be parsed as SKILL.md.
							</AlertDescription>
						</Alert>
					)}
					<DialogFooter>
						<Button variant="outline" onClick={state.onClose}>
							Close
						</Button>
						<Button onClick={state.onRetry} disabled={state.isRetrying}>
							{state.isRetrying && <Spinner className="size-4" loading />}
							Retry
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		);
	}

	return (
		<SkillEditor
			open
			mode="edit"
			readOnly={state.readOnly}
			noun={copy.noun}
			description={copy.editorDescription}
			initialValues={state.initialValues}
			existingNames={state.existingNames}
			submitError={state.submitError}
			isSubmitting={state.isSubmitting}
			onOpenChange={handleOpenChange}
			onCloseAutoFocus={onCloseAutoFocus}
			onSubmit={state.onSubmit}
		/>
	);
};

export const DeleteSkillDialog: React.FC<
	DialogFocusProps & { state: SkillDeleteState }
> = ({ state, onCloseAutoFocus }) => {
	return (
		<ConfirmDialog
			type="delete"
			open
			onClose={state.onClose}
			onCloseAutoFocus={onCloseAutoFocus}
			title="Delete skill"
			confirmText="Delete skill"
			description={
				<>
					<p className="m-0">
						Delete {state.skill.name}? Agents will no longer be able to use this
						skill. This action cannot be undone.
					</p>
					{state.error && (
						<Alert severity="error" className="mt-3">
							<AlertDescription>
								{state.error.message}
								{state.error.detail ? ` ${state.error.detail}` : ""}
							</AlertDescription>
						</Alert>
					)}
				</>
			}
			onConfirm={state.onConfirm}
			confirmLoading={state.isDeleting}
		/>
	);
};
