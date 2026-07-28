import type { FC } from "react";
import type { UserSkillMetadata } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { ConfirmDeleteDialog } from "#/components/Dialogs/ConfirmDeleteDialog/ConfirmDeleteDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { formatDate } from "#/utils/time";
import type { PersonalSkillErrorDisplay } from "./components/PersonalSkillEditor";
import { PersonalSkillEditor } from "./components/PersonalSkillEditor";
import { SectionHeader } from "./components/SectionHeader";
import {
	PERSONAL_SKILLS_MAX_PER_USER,
	type PersonalSkillFormValues,
} from "./utils/personalSkills";

export type PersonalSkillEditorState =
	| {
			mode: "create";
			initialValues: PersonalSkillFormValues;
			existingNames: readonly string[];
			submitError?: PersonalSkillErrorDisplay;
			isSubmitting: boolean;
			onSubmit: (values: PersonalSkillFormValues, content: string) => void;
			onClose: () => void;
	  }
	| {
			mode: "edit";
			initialValues?: PersonalSkillFormValues;
			existingNames: readonly string[];
			loadError?: unknown;
			isLoading: boolean;
			isRetrying: boolean;
			submitError?: PersonalSkillErrorDisplay;
			isSubmitting: boolean;
			onRetry: () => void;
			onSubmit: (values: PersonalSkillFormValues, content: string) => void;
			onClose: () => void;
	  };

export type PersonalSkillDeleteState = {
	skill: UserSkillMetadata;
	error?: PersonalSkillErrorDisplay;
	isDeleting: boolean;
	onConfirm: () => void;
	onClose: () => void;
};

export interface AgentSettingsPersonalSkillsPageViewProps {
	skills: readonly UserSkillMetadata[];
	error: unknown;
	isLoading: boolean;
	isRetrying: boolean;
	onRetry: () => void;
	onCreate: () => void;
	onEdit: (name: string) => void;
	onDelete: (skill: UserSkillMetadata) => void;
	editorState?: PersonalSkillEditorState;
	deleteState?: PersonalSkillDeleteState;
}

const formatUpdatedAt = (value: string) => {
	const date = new Date(value);
	if (!Number.isFinite(date.getTime())) {
		return "Unknown";
	}
	return formatDate(date, {
		locale: "en-US",
		month: "short",
		day: "numeric",
		year: "numeric",
		hour: "numeric",
		second: undefined,
		minute: "2-digit",
	});
};

const EditSkillDialog: FC<{
	state: Extract<PersonalSkillEditorState, { mode: "edit" }>;
}> = ({ state }) => {
	const handleOpenChange = (open: boolean) => {
		if (!open) {
			state.onClose();
		}
	};

	if (state.isLoading) {
		return (
			<Dialog open onOpenChange={handleOpenChange}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Loading personal skill</DialogTitle>
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
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Unable to load personal skill</DialogTitle>
						<DialogDescription>
							The skill could not be loaded for editing.
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
		<PersonalSkillEditor
			open
			mode="edit"
			initialValues={state.initialValues}
			existingNames={state.existingNames}
			submitError={state.submitError}
			isSubmitting={state.isSubmitting}
			onOpenChange={handleOpenChange}
			onSubmit={state.onSubmit}
		/>
	);
};

const DeleteSkillDialog: FC<{ state: PersonalSkillDeleteState }> = ({
	state,
}) => {
	const handleOpenChange = (open: boolean) => {
		if (!open) {
			state.onClose();
		}
	};

	return (
		<ConfirmDeleteDialog
			open
			onOpenChange={handleOpenChange}
			entity="skill"
			description={
				<>
					Delete {state.skill.name}? Agents will no longer be able to use this
					skill. This action cannot be undone.
				</>
			}
			onConfirm={state.onConfirm}
			isPending={state.isDeleting}
		>
			{state.error && (
				<Alert severity="error">
					<AlertDescription>
						{state.error.message}
						{state.error.detail ? ` ${state.error.detail}` : ""}
					</AlertDescription>
				</Alert>
			)}
		</ConfirmDeleteDialog>
	);
};

export const AgentSettingsPersonalSkillsPageView: FC<
	AgentSettingsPersonalSkillsPageViewProps
> = ({
	skills,
	error,
	isLoading,
	isRetrying,
	onRetry,
	onCreate,
	onEdit,
	onDelete,
	editorState,
	deleteState,
}) => {
	const isAtLimit = skills.length >= PERSONAL_SKILLS_MAX_PER_USER;
	const addSkillAction = (
		<Button size="sm" onClick={onCreate} disabled={isLoading || isAtLimit}>
			Add skill
		</Button>
	);

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Personal skills"
				description="Reusable instructions your agents can pick when they need specialized guidance. Personal skills hold a single SKILL.md file. For richer skills with supporting files, add them to your repo under `.agents/skills/` or load them from a workspace."
				action={addSkillAction}
			/>

			{isAtLimit && (
				<Alert severity="warning">
					<AlertDescription>
						You have reached the limit of {PERSONAL_SKILLS_MAX_PER_USER}{" "}
						personal skills. Delete a skill before creating another one.
					</AlertDescription>
				</Alert>
			)}

			{error ? (
				<div className="flex flex-col items-start gap-3">
					<ErrorAlert error={error} />
					<Button
						variant="outline"
						size="sm"
						onClick={onRetry}
						disabled={isRetrying}
					>
						{isRetrying && <Spinner className="size-4" loading />}
						Retry
					</Button>
				</div>
			) : isLoading ? (
				<Loader />
			) : skills.length === 0 ? (
				<EmptyState
					message="No personal skills yet"
					description="Create a personal skill to save reusable agent guidance for your workflows."
					cta={addSkillAction}
				/>
			) : (
				<Table aria-label="Personal skills">
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Description</TableHead>
							<TableHead>Updated</TableHead>
							<TableHead className="text-right">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{skills.map((skill) => (
							<TableRow key={skill.id}>
								<TableCell className="font-mono text-content-primary">
									{skill.name}
								</TableCell>
								<TableCell>
									{skill.description || (
										<span className="text-content-secondary">
											No description
										</span>
									)}
								</TableCell>
								<TableCell>{formatUpdatedAt(skill.updated_at)}</TableCell>
								<TableCell>
									<div className="flex justify-end gap-2">
										<Button
											size="xs"
											variant="outline"
											onClick={() => onEdit(skill.name)}
										>
											Edit
										</Button>
										<Button
											size="xs"
											variant="destructive"
											onClick={() => onDelete(skill)}
										>
											Delete
										</Button>
									</div>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}

			{editorState?.mode === "create" && (
				<PersonalSkillEditor
					open
					mode="create"
					initialValues={editorState.initialValues}
					existingNames={editorState.existingNames}
					submitError={editorState.submitError}
					isSubmitting={editorState.isSubmitting}
					onOpenChange={(open) => {
						if (!open) {
							editorState.onClose();
						}
					}}
					onSubmit={editorState.onSubmit}
				/>
			)}
			{editorState?.mode === "edit" && <EditSkillDialog state={editorState} />}
			{deleteState && <DeleteSkillDialog state={deleteState} />}
		</div>
	);
};
