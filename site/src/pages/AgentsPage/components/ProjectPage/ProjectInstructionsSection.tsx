import { useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import {
	chatProjectInstructions,
	deleteChatProjectInstructions,
	updateChatProjectInstructions,
} from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectInstructions,
} from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { InvisibleCharactersAlert } from "#/components/Alert/InvisibleCharactersAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { sanitizePromptText } from "#/utils/invisibleUnicode";
import {
	SectionError,
	SectionLoadState,
	SectionSaveError,
	SettingsCard,
	SettingsCardRow,
	SettingsSection,
} from "./ProjectSection";
import { useSingleFlight } from "./useSingleFlight";

type ProjectInstructionsSectionProps = {
	readonly project: ChatProject;
};

/** Loads an agent's instructions and saves edits to them. */
export const ProjectInstructionsSection: React.FC<
	ProjectInstructionsSectionProps
> = ({ project }) => {
	const queryClient = useQueryClient();
	const instructionsQuery = useQuery(chatProjectInstructions(project));
	const updateMutation = useMutation(
		updateChatProjectInstructions(queryClient, project),
	);
	const deleteMutation = useMutation(
		deleteChatProjectInstructions(queryClient, project),
	);
	const singleFlight = useSingleFlight();
	const [draft, setDraft] = useState<ChatProjectInstructionsDraft>();

	const resetErrors = () => {
		updateMutation.reset();
		deleteMutation.reset();
	};

	const runMutation = (mutate: (onSettled: () => void) => void) => {
		singleFlight.run((onSettled) => {
			// Clear the previous action's error so only the latest one shows.
			resetErrors();
			mutate(onSettled);
		});
	};

	return (
		<ProjectInstructionsSectionView
			instructions={instructionsQuery.data}
			loadError={instructionsQuery.error}
			onRetry={() => void instructionsQuery.refetch()}
			canEdit={project.permissions.edit}
			draft={draft}
			onDraftChange={(next) => {
				setDraft(next);
				if (!singleFlight.isInFlight()) {
					resetErrors();
				}
			}}
			isSaving={updateMutation.isPending}
			isDeleting={deleteMutation.isPending}
			saveError={updateMutation.error}
			deleteError={deleteMutation.error}
			onSave={(text) =>
				runMutation((onSettled) =>
					updateMutation.mutate(
						{ instructions: text },
						{ onSuccess: () => setDraft(undefined), onSettled },
					),
				)
			}
			onDelete={() =>
				runMutation((onSettled) =>
					deleteMutation.mutate(undefined, {
						onSuccess: () => setDraft(undefined),
						onSettled,
					}),
				)
			}
		/>
	);
};

/**
 * An edit in progress. `base` is the saved text the edit started from, so a
 * different saved text means someone else changed the instructions meanwhile.
 */
type ChatProjectInstructionsDraft = {
	readonly text: string;
	readonly base: string;
};

type ProjectInstructionsSectionViewProps = {
	/** `undefined` until the instructions load. */
	readonly instructions: ChatProjectInstructions | undefined;
	readonly loadError: unknown;
	readonly onRetry: () => void;
	readonly canEdit: boolean;
	/** `undefined` while the text on screen is the saved text. */
	readonly draft: ChatProjectInstructionsDraft | undefined;
	readonly onDraftChange: (
		draft: ChatProjectInstructionsDraft | undefined,
	) => void;
	readonly isSaving: boolean;
	readonly isDeleting: boolean;
	readonly saveError: unknown;
	readonly deleteError: unknown;
	readonly onSave: (text: string) => void;
	readonly onDelete: () => void;
};

/**
 * A large text card for the instructions added to every chat in the agent.
 * Save and Discard appear while there are unsaved changes. Clearing the text
 * offers to delete the instructions instead.
 */
export const ProjectInstructionsSectionView: React.FC<
	ProjectInstructionsSectionViewProps
> = ({
	instructions,
	loadError,
	onRetry,
	canEdit,
	draft,
	onDraftChange,
	isSaving,
	isDeleting,
	saveError,
	deleteError,
	onSave,
	onDelete,
}) => {
	const textareaId = useId();
	const textareaRef = useRef<HTMLTextAreaElement>(null);
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);

	if (instructions === undefined) {
		return (
			<SettingsSection title="Instructions">
				<SectionLoadState error={loadError} onRetry={onRetry} />
			</SettingsSection>
		);
	}

	const saved = instructions.instructions;
	const text = draft?.text ?? saved;
	const hasConflict = draft !== undefined && draft.base !== saved;
	const normalizedDraft = sanitizePromptText(text);
	const isDirty =
		draft !== undefined && normalizedDraft !== sanitizePromptText(saved);
	const isBusy = isSaving || isDeleting;
	const isClearing = isDirty && normalizedDraft === "";
	const error = saveError ?? deleteError;

	const actions = isDirty && (
		<div className="flex items-center gap-2">
			<Button
				type="button"
				size="sm"
				variant="outline"
				disabled={isBusy}
				onClick={() => {
					onDraftChange(undefined);
					textareaRef.current?.focus();
				}}
			>
				Discard
			</Button>
			{isClearing ? (
				<Button
					type="button"
					size="sm"
					variant="destructive"
					disabled={isBusy || hasConflict}
					onClick={() => setIsConfirmingDelete(true)}
				>
					<Spinner loading={isDeleting} />
					Delete
				</Button>
			) : (
				<Button
					type="button"
					size="sm"
					disabled={isBusy || hasConflict}
					onClick={() => {
						onSave(text);
						textareaRef.current?.focus();
					}}
				>
					<Spinner loading={isSaving} />
					Save
				</Button>
			)}
		</div>
	);

	return (
		<SettingsSection
			title="Instructions"
			description="Added to every chat in this agent, for all users."
			actions={actions}
		>
			{Boolean(loadError) && (
				<SectionError error={loadError} onRetry={onRetry} />
			)}
			{Boolean(error) && (
				<SectionSaveError
					error={error}
					fallback={
						saveError
							? "Failed to save instructions."
							: "Failed to delete instructions."
					}
				/>
			)}
			<SettingsCard as="div">
				<SettingsCardRow as="div" className="p-0">
					{canEdit ? (
						<>
							<label htmlFor={textareaId} className="sr-only">
								Instructions
							</label>
							<TextareaAutosize
								id={textareaId}
								ref={textareaRef}
								className="max-h-[50vh] w-full resize-none overflow-y-auto border-0 bg-transparent px-4 py-3 font-sans text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link scrollbar-thin"
								placeholder="Conventions, context, and preferences for every chat in this agent"
								value={text}
								onChange={(event) =>
									onDraftChange({
										text: event.target.value,
										base: draft?.base ?? saved,
									})
								}
								readOnly={isBusy}
								minRows={6}
							/>
						</>
					) : (
						<p className="m-0 w-full whitespace-pre-wrap break-words px-4 py-3 text-sm text-content-primary">
							{saved === "" ? (
								<span className="text-content-secondary">
									No instructions yet.
								</span>
							) : (
								saved
							)}
						</p>
					)}
				</SettingsCardRow>
			</SettingsCard>
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
								onClick={() => {
									onDraftChange({ text, base: saved });
									textareaRef.current?.focus();
								}}
							>
								Keep my draft
							</Button>
							<Button
								type="button"
								size="sm"
								disabled={isBusy}
								onClick={() => {
									onDraftChange(undefined);
									textareaRef.current?.focus();
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
			<InvisibleCharactersAlert text={text} />
			<ConfirmDialog
				open={isConfirmingDelete}
				type="delete"
				title="Delete instructions?"
				description="Chats in this agent will no longer receive these instructions, for all users. This action cannot be undone."
				confirmText="Delete instructions"
				onClose={() => setIsConfirmingDelete(false)}
				onConfirm={() => {
					setIsConfirmingDelete(false);
					onDelete();
					// The dialog returns focus to Delete, which is about to go away.
					requestAnimationFrame(() => textareaRef.current?.focus());
				}}
			/>
		</SettingsSection>
	);
};
