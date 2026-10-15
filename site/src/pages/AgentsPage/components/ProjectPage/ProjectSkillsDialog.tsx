import { isAxiosError } from "axios";
import { ChevronLeftIcon, EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { ChatProject, SkillMetadata } from "#/api/typesGenerated";
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
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
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
import { toast } from "#/components/Toaster/toast";
import { DATE_FORMAT, formatDateTime, relativeTime } from "#/utils/time";
import {
	emptySkillFormValues,
	projectSkillsScope,
	type SkillsScope,
	skillFormValuesFromSkill,
} from "../../utils/skills";
import { RetryButton } from "../RetryButton";
import { SkillEditorForm, type SkillErrorDisplay } from "../SkillEditor";
import { downloadSkillFile, skillError } from "../SkillsTable";
import { DeleteSkillDialog } from "../SkillsTableView";
import { useDialogFocusReturn } from "../useDialogFocusReturn";

const NOUN = "project skill";

type EditorView = { type: "create" } | { type: "edit"; name: string };

type ProjectSkillsDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly canEdit: boolean;
	readonly onCloseAutoFocus: (event: Event) => void;
};

/** Lists, creates, edits, and deletes the skills of a chat project. */
export const ProjectSkillsDialog: React.FC<ProjectSkillsDialogProps> = ({
	project,
	open,
	onOpenChange,
	canEdit,
	onCloseAutoFocus,
}) => {
	const queryClient = useQueryClient();
	const scope = projectSkillsScope(project.organization_id, project.id);
	const [editor, setEditor] = useState<EditorView | null>(null);
	// The list remounts after the editor closes, so it autofocuses the
	// control that opened the editor.
	const [lastEditor, setLastEditor] = useState<EditorView | null>(null);
	const [skillToDelete, setSkillToDelete] = useState<SkillMetadata | null>(
		null,
	);
	const { rememberTrigger, restoreFocus, setPrimaryFallback } =
		useDialogFocusReturn();

	const skillsQuery = useQuery(scope.list);
	const skills = skillsQuery.data ?? [];
	const createMutation = useMutation(scope.create(queryClient));
	const updateMutation = useMutation(scope.update(queryClient));
	const deleteMutation = useMutation(scope.remove(queryClient));
	const downloadMutation = useMutation({
		mutationFn: (name: string) =>
			downloadSkillFile(name, (skillName) =>
				queryClient
					.fetchQuery(scope.detail(skillName))
					.then((skill) => skill.content),
			),
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to download ${NOUN}.`), {
				description: getErrorDetail(error),
			});
		},
	});
	const isSubmitting = createMutation.isPending || updateMutation.isPending;

	const openEditor = (view: EditorView) => {
		createMutation.reset();
		updateMutation.reset();
		setEditor(view);
	};
	const closeEditor = () => {
		setLastEditor(editor);
		setEditor(null);
	};
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !isSubmitting) {
			setEditor(null);
			setLastEditor(null);
			onOpenChange(false);
		}
	};

	const editorMutation =
		editor?.type === "create" ? createMutation : updateMutation;

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent
				className="flex max-w-2xl flex-col gap-0 overflow-hidden p-0"
				onCloseAutoFocus={onCloseAutoFocus}
			>
				{editor ? (
					<ProjectSkillEditorView
						scope={scope}
						view={editor}
						canEdit={canEdit}
						existingNames={skills.map((skill) =>
							skill.name.toLocaleLowerCase("en-US"),
						)}
						submitError={skillError(
							editorMutation.error,
							`Failed to ${editor.type === "create" ? "create" : "save"} ${NOUN}.`,
							NOUN,
						)}
						isSubmitting={isSubmitting}
						onBack={closeEditor}
						onSubmit={(content) => {
							const onSuccess = () => {
								closeEditor();
								toast.success(
									`Project skill ${editor.type === "create" ? "created" : "saved"}.`,
								);
							};
							if (editor.type === "create") {
								createMutation.mutate({ content }, { onSuccess });
							} else {
								updateMutation.mutate(
									{ name: editor.name, req: { content } },
									{ onSuccess },
								);
							}
						}}
					/>
				) : (
					<>
						<div className="flex items-start justify-between gap-4 px-6 pt-6">
							<DialogHeader>
								<DialogTitle>Project skills</DialogTitle>
								<DialogDescription>
									Every chat in this project can use these.
								</DialogDescription>
							</DialogHeader>
							{canEdit && (
								<Button
									ref={setPrimaryFallback}
									size="sm"
									variant="outline"
									autoFocus={lastEditor?.type === "create"}
									onClick={() => openEditor({ type: "create" })}
								>
									<PlusIcon />
									New
								</Button>
							)}
						</div>
						<div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-4">
							{skillsQuery.error ? (
								<ErrorAlert
									error={skillsQuery.error}
									actions={
										<RetryButton
											size="sm"
											variant="outline"
											isRetrying={skillsQuery.isFetching}
											onRetry={() => void skillsQuery.refetch()}
										/>
									}
								/>
							) : null}
							{skillsQuery.isLoading && <Loader />}
							{skillsQuery.isSuccess && skills.length === 0 && (
								<div className="rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-content-secondary">
									No skills added yet
								</div>
							)}
							{skills.length > 0 && (
								<Table aria-label="Project skills">
									<TableHeader>
										<TableRow>
											<TableHead className="w-full">Skill</TableHead>
											<TableHead className="whitespace-nowrap">
												Last updated
											</TableHead>
											<TableHead className="w-14">
												<span className="sr-only">Actions</span>
											</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{skills.map((skill) => (
											<ProjectSkillRow
												key={skill.id}
												skill={skill}
												canEdit={canEdit}
												autoFocus={
													lastEditor?.type === "edit" &&
													lastEditor.name === skill.name
												}
												isDownloading={
													downloadMutation.isPending &&
													downloadMutation.variables === skill.name
												}
												onTriggerInteraction={rememberTrigger}
												onEdit={() =>
													openEditor({ type: "edit", name: skill.name })
												}
												onDownload={() => downloadMutation.mutate(skill.name)}
												onDelete={() => {
													deleteMutation.reset();
													setSkillToDelete(skill);
												}}
											/>
										))}
									</TableBody>
								</Table>
							)}
						</div>
						<DialogFooter className="border-t border-border px-6 py-4">
							<Button variant="outline" onClick={() => handleOpenChange(false)}>
								Close
							</Button>
						</DialogFooter>
					</>
				)}
				{skillToDelete && (
					<DeleteSkillDialog
						state={{
							skill: skillToDelete,
							error: skillError(
								deleteMutation.error,
								`Failed to delete ${NOUN}.`,
								NOUN,
							),
							isDeleting: deleteMutation.isPending,
							onConfirm: () =>
								deleteMutation.mutate(skillToDelete.name, {
									onSuccess: () => {
										setSkillToDelete(null);
										toast.success("Project skill deleted.");
									},
									onError: (error) => {
										// Someone else deleted it, and the list refetch drops it.
										if (isAxiosError(error) && error.response?.status === 404) {
											setSkillToDelete(null);
										}
									},
								}),
							onClose: () => {
								if (!deleteMutation.isPending) {
									setSkillToDelete(null);
								}
							},
						}}
						onCloseAutoFocus={restoreFocus}
					/>
				)}
			</DialogContent>
		</Dialog>
	);
};

type ProjectSkillEditorViewProps = {
	readonly scope: SkillsScope;
	readonly view: EditorView;
	readonly canEdit: boolean;
	readonly existingNames: readonly string[];
	readonly submitError: SkillErrorDisplay | undefined;
	readonly isSubmitting: boolean;
	readonly onBack: () => void;
	readonly onSubmit: (content: string) => void;
};

const ProjectSkillEditorView: React.FC<ProjectSkillEditorViewProps> = ({
	scope,
	view,
	canEdit,
	existingNames,
	submitError,
	isSubmitting,
	onBack,
	onSubmit,
}) => {
	const skillQuery = useQuery({
		...scope.detail(view.type === "edit" ? view.name : ""),
		enabled: view.type === "edit",
	});
	let loaded: ReturnType<typeof skillFormValuesFromSkill> | undefined;
	if (view.type === "create") {
		loaded = { ok: true, values: emptySkillFormValues };
	} else if (skillQuery.data) {
		loaded = skillFormValuesFromSkill(skillQuery.data);
	}
	const title =
		view.type === "create"
			? "New skill"
			: canEdit
				? "Edit skill"
				: "View skill";
	const header = (
		<DialogHeader className="items-start px-6 pt-6">
			<Button
				size="sm"
				variant="subtle"
				className="-ml-2"
				disabled={isSubmitting}
				autoFocus
				onClick={onBack}
			>
				<ChevronLeftIcon />
				Back
			</Button>
			<DialogTitle>{title}</DialogTitle>
			<DialogDescription>
				Every chat in this project can use this skill.
			</DialogDescription>
		</DialogHeader>
	);

	if (!loaded?.ok) {
		const loadError = loaded ? new Error(loaded.error) : skillQuery.error;
		return (
			<>
				{header}
				<div className="px-6 py-6">
					{loadError ? (
						<ErrorAlert
							error={loadError}
							actions={
								<RetryButton
									size="sm"
									variant="outline"
									isRetrying={skillQuery.isFetching}
									onRetry={() => void skillQuery.refetch()}
								/>
							}
						/>
					) : (
						<Loader />
					)}
				</div>
			</>
		);
	}

	return (
		<SkillEditorForm
			mode={view.type}
			readOnly={!canEdit}
			initialValues={loaded.values}
			existingNames={existingNames}
			submitError={submitError}
			isSubmitting={isSubmitting}
			nameHint="Lowercase letters, numbers, and hyphens."
			bodyPlaceholder="Describe when and how the agent should use this skill."
			header={header}
			footer={({ canSubmit }) =>
				canEdit && (
					<DialogFooter className="border-t border-border px-6 py-4">
						<Button type="submit" disabled={!canSubmit}>
							<Spinner loading={isSubmitting} />
							Save
						</Button>
					</DialogFooter>
				)
			}
			onSubmit={(_values, content) => onSubmit(content)}
		/>
	);
};

type ProjectSkillRowProps = {
	readonly skill: SkillMetadata;
	readonly canEdit: boolean;
	readonly autoFocus: boolean;
	readonly isDownloading: boolean;
	readonly onTriggerInteraction: (
		event: React.SyntheticEvent<HTMLButtonElement>,
	) => void;
	readonly onEdit: () => void;
	readonly onDownload: () => void;
	readonly onDelete: () => void;
};

const ProjectSkillRow: React.FC<ProjectSkillRowProps> = ({
	skill,
	canEdit,
	autoFocus,
	isDownloading,
	onTriggerInteraction,
	onEdit,
	onDownload,
	onDelete,
}) => (
	<TableRow>
		<TableCell className="w-full max-w-0">
			<div className="truncate font-medium text-content-primary">
				{skill.name}
			</div>
			<div
				className="truncate text-xs text-content-secondary"
				title={skill.description || undefined}
			>
				{skill.description || "No description"}
			</div>
		</TableCell>
		<TableCell className="whitespace-nowrap text-content-secondary">
			<time
				dateTime={skill.updated_at}
				title={formatDateTime(skill.updated_at, DATE_FORMAT.FULL_DATETIME)}
			>
				{relativeTime(skill.updated_at)}
			</time>
		</TableCell>
		<TableCell className="text-right">
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						size="icon"
						variant="subtle"
						aria-label={`Open menu for ${skill.name}`}
						autoFocus={autoFocus}
						onPointerDown={onTriggerInteraction}
						onKeyDown={onTriggerInteraction}
					>
						{isDownloading ? (
							<Spinner loading />
						) : (
							<EllipsisVerticalIcon aria-hidden="true" />
						)}
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem onClick={onEdit}>
						{canEdit ? "Edit" : "View"}
					</DropdownMenuItem>
					<DropdownMenuItem onClick={onDownload} disabled={isDownloading}>
						Download
					</DropdownMenuItem>
					{canEdit && (
						<>
							<DropdownMenuSeparator />
							<DropdownMenuItem
								className="text-content-destructive focus:text-content-destructive"
								onClick={onDelete}
							>
								Delete&hellip;
							</DropdownMenuItem>
						</>
					)}
				</DropdownMenuContent>
			</DropdownMenu>
		</TableCell>
	</TableRow>
);
