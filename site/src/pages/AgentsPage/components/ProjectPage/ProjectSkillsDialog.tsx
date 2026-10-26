import { isAxiosError } from "axios";
import { ChevronLeftIcon, EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { ChatProject, SkillMetadata } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
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
	emptyProjectSkillFormValues,
	projectSkillsScope,
	SKILLS_MAX_PER_OWNER,
	type SkillFormValues,
	type SkillsScope,
	skillFormValuesFromSkill,
	withDisabledModelInvocation,
} from "../../utils/skills";
import { RetryButton } from "../RetryButton";
import {
	SkillEditorForm,
	type SkillErrorDisplay,
	type SkillSubmitHandler,
} from "../SkillEditor";
import { SkillEnabledSwitch } from "../SkillEnabledSwitch";
import { downloadSkillFile, skillError } from "../SkillsTable";
import { DeleteSkillDialog } from "../SkillsTableView";
import { useDialogFocusReturn } from "../useDialogFocusReturn";

const NOUN = "project skill";

type EditorView = { type: "create" } | { type: "edit"; name: string };

const editorSubmitError = (
	view: EditorView,
	error: unknown,
): SkillErrorDisplay | undefined => {
	if (
		view.type === "edit" &&
		isAxiosError(error) &&
		error.response?.status === 404
	) {
		return {
			message: "This skill was deleted. Go back to see the current list.",
		};
	}
	return skillError(
		error,
		`Failed to ${view.type === "create" ? "create" : "save"} ${NOUN}.`,
		NOUN,
	);
};

type ProjectSkillsDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly canEdit: boolean;
	readonly onCloseAutoFocus: (event: Event) => void;
};

/** Manages project skills, read-only without canEdit; onCloseAutoFocus restores focus. */
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
	// The list remounts after the editor closes and autofocuses its opener.
	const [lastEditor, setLastEditor] = useState<EditorView | null>(null);
	const [skillToDelete, setSkillToDelete] = useState<SkillMetadata | null>(
		null,
	);
	const { rememberTrigger, restoreFocus, setPrimaryFallback } =
		useDialogFocusReturn();
	const backButtonRef = useRef<HTMLButtonElement>(null);

	const skillsQuery = useQuery(scope.list);
	const skills = skillsQuery.data ?? [];
	const isAtLimit = skills.length >= SKILLS_MAX_PER_OWNER;
	const createMutation = useMutation(scope.create(queryClient));
	const updateMutation = useMutation(scope.update(queryClient));
	const deleteMutation = useMutation(scope.remove(queryClient));
	const downloadMutation = useMutation({
		mutationFn: (name: string) =>
			downloadSkillFile(name, (skillName) =>
				queryClient
					.fetchQuery(scope.detail(skillName))
					.then((skill) =>
						skill.model_invocable
							? skill.content
							: withDisabledModelInvocation(skill.content),
					),
			),
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to download ${NOUN}.`), {
				description: getErrorDetail(error),
			});
			// Someone else deleted it, and the list refetch drops its row.
			if (isAxiosError(error) && error.response?.status === 404) {
				void skillsQuery.refetch();
			}
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

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent
				className="flex max-w-2xl flex-col gap-0 overflow-hidden p-0"
				onCloseAutoFocus={onCloseAutoFocus}
				onEscapeKeyDown={(event) => {
					// Back asks before it discards unsaved edits.
					if (editor && canEdit) {
						event.preventDefault();
						backButtonRef.current?.click();
					}
				}}
				onInteractOutside={(event) => {
					if (editor && canEdit) {
						event.preventDefault();
					}
				}}
			>
				{editor ? (
					<ProjectSkillEditorView
						scope={scope}
						view={editor}
						canEdit={canEdit}
						existingNames={skills.map((skill) =>
							skill.name.toLocaleLowerCase("en-US"),
						)}
						submitError={editorSubmitError(
							editor,
							(editor.type === "create" ? createMutation : updateMutation)
								.error,
						)}
						isSubmitting={isSubmitting}
						backButtonRef={backButtonRef}
						onBack={closeEditor}
						onSubmit={(values, content, onSettled, isContentChanged) => {
							const req = {
								content,
								action: values.action,
								model_invocable: values.modelInvocable,
							};
							const onSuccess = () => {
								closeEditor();
								toast.success(
									`Project skill ${editor.type === "create" ? "created" : "saved"}.`,
								);
							};
							if (editor.type === "create") {
								createMutation.mutate(req, { onSuccess, onSettled });
							} else {
								updateMutation.mutate(
									{
										name: editor.name,
										req: isContentChanged
											? req
											: { ...req, content: undefined },
									},
									{ onSuccess, onSettled },
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
									{isAtLimit
										? `You have reached the limit of ${SKILLS_MAX_PER_OWNER} ${NOUN}s. Delete a skill before creating another one.`
										: "Every chat in this project can use these."}
								</DialogDescription>
							</DialogHeader>
							{canEdit && (
								<Button
									ref={setPrimaryFallback}
									size="sm"
									variant="outline"
									autoFocus={lastEditor?.type === "create"}
									disabled={isAtLimit || skillsQuery.isLoading}
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
												Enabled
											</TableHead>
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
												scope={scope}
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
	readonly backButtonRef: React.RefObject<HTMLButtonElement | null>;
	readonly onBack: () => void;
	readonly onSubmit: (
		...args: [...Parameters<SkillSubmitHandler>, isContentChanged: boolean]
	) => void;
};

const ProjectSkillEditorView: React.FC<ProjectSkillEditorViewProps> = ({
	scope,
	view,
	canEdit,
	existingNames,
	submitError,
	isSubmitting,
	backButtonRef,
	onBack,
	onSubmit,
}) => {
	const queryClient = useQueryClient();
	const [isDiscardOpen, setIsDiscardOpen] = useState(false);
	const skillQuery = useQuery({
		...scope.detail(view.type === "edit" ? view.name : ""),
		enabled: view.type === "edit",
	});
	const isSkillDeleted =
		view.type === "edit" &&
		isAxiosError(skillQuery.error) &&
		skillQuery.error.response?.status === 404;
	let loaded: ReturnType<typeof skillFormValuesFromSkill> | undefined;
	if (view.type === "create") {
		loaded = { ok: true, values: emptyProjectSkillFormValues };
	} else if (skillQuery.data) {
		loaded = skillFormValuesFromSkill(skillQuery.data);
	}
	// Editing keeps the first loaded values so a refetch cannot reset edits.
	const [openedValues, setOpenedValues] = useState<SkillFormValues>();
	if (loaded?.ok && canEdit && !openedValues) {
		setOpenedValues(loaded.values);
	}
	// A detail 404 does not refresh the separate list query, so invalidate it.
	useEffect(() => {
		if (!isSkillDeleted) {
			return;
		}
		toast.info(`That ${NOUN} was deleted.`);
		void queryClient.invalidateQueries({ ...scope.list, exact: true });
		onBack();
	}, [isSkillDeleted, onBack, queryClient, scope.list]);
	if (isSkillDeleted) {
		return null;
	}
	const title =
		view.type === "create"
			? "New skill"
			: canEdit
				? "Edit skill"
				: "View skill";

	if (!loaded?.ok) {
		const loadError = loaded ? new Error(loaded.error) : skillQuery.error;
		return (
			<>
				<ProjectSkillEditorHeader
					title={title}
					isSubmitting={isSubmitting}
					backButtonRef={backButtonRef}
					onBack={onBack}
				/>
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
		<>
			<SkillEditorForm
				mode={view.type}
				readOnly={!canEdit}
				initialValues={openedValues ?? loaded.values}
				existingNames={existingNames}
				submitError={submitError}
				isSubmitting={isSubmitting}
				nameHint="Lowercase letters, numbers, and hyphens."
				bodyPlaceholder="Describe when and how the agent should use this skill."
				showProjectSettings
				header={({ isDirty }) => (
					<ProjectSkillEditorHeader
						title={title}
						isSubmitting={isSubmitting}
						backButtonRef={backButtonRef}
						onBack={isDirty ? () => setIsDiscardOpen(true) : onBack}
					/>
				)}
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
				onSubmit={(values, content, onSettled) => {
					// A settings-only edit keeps the stored SKILL.md as written.
					const opened = openedValues ?? loaded.values;
					onSubmit(
						values,
						content,
						onSettled,
						view.type === "create" ||
							values.name !== opened.name ||
							values.description !== opened.description.trim() ||
							values.body !== opened.body,
					);
				}}
			/>
			<ConfirmDialog
				open={isDiscardOpen}
				type="delete"
				title="Discard unsaved changes?"
				description="Your unsaved changes will be lost."
				cancelText="Keep editing"
				confirmText="Discard"
				onConfirm={onBack}
				onClose={() => setIsDiscardOpen(false)}
				onCloseAutoFocus={(event) => {
					if (backButtonRef.current?.isConnected) {
						event.preventDefault();
						backButtonRef.current.focus();
					}
				}}
			/>
		</>
	);
};

type ProjectSkillEditorHeaderProps = {
	readonly title: string;
	readonly isSubmitting: boolean;
	readonly backButtonRef: React.RefObject<HTMLButtonElement | null>;
	readonly onBack: () => void;
};

const ProjectSkillEditorHeader: React.FC<ProjectSkillEditorHeaderProps> = ({
	title,
	isSubmitting,
	backButtonRef,
	onBack,
}) => (
	<DialogHeader className="items-start px-6 pt-6">
		<Button
			ref={backButtonRef}
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

type ProjectSkillRowProps = {
	readonly scope: SkillsScope;
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
	scope,
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
		<TableCell className="w-full max-w-0" title={skill.name}>
			<div className="flex items-center gap-1.5">
				<span className="truncate font-medium text-content-primary">
					{skill.name}
				</span>
				{skill.action && <Badge size="sm">Action</Badge>}
				{!skill.model_invocable && <Badge size="sm">Manual only</Badge>}
			</div>
			<div
				className="truncate text-xs text-content-secondary"
				title={skill.description || undefined}
			>
				{skill.description || "No description"}
			</div>
		</TableCell>
		<TableCell>
			<SkillEnabledSwitch scope={scope} skill={skill} readOnly={!canEdit} />
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
					<DropdownMenuItem onClick={onDownload} disabled={isDownloading}>
						Download
					</DropdownMenuItem>
					<DropdownMenuItem onClick={onEdit}>
						{canEdit ? "Edit" : "View"}
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
