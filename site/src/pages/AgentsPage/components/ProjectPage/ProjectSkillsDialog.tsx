import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
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
	projectSkillsScope,
	type SkillFormValues,
	tryParseSkillMarkdown,
} from "../../utils/skills";
import { RetryButton } from "../RetryButton";
import { SkillEditorForm } from "../SkillEditor";
import { downloadSkillFile, skillError } from "../SkillsTable";
import { DeleteSkillDialog } from "../SkillsTableView";
import { useDialogFocusReturn } from "../useDialogFocusReturn";

const NOUN = "project skill";

const emptySkillFormValues: SkillFormValues = {
	name: "",
	description: "",
	body: "",
};

type EditorView = { type: "create" } | { type: "edit"; name: string };

type ProjectSkillsDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	/** Without update access on the project, skills can only be viewed. */
	readonly canEdit: boolean;
};

/** Lists, creates, edits, and deletes the skills of a chat project. */
export const ProjectSkillsDialog: React.FC<ProjectSkillsDialogProps> = ({
	project,
	open,
	onOpenChange,
	canEdit,
}) => {
	const queryClient = useQueryClient();
	const scope = projectSkillsScope(project.organization_id, project.id);
	const [editor, setEditor] = useState<EditorView | null>(null);
	const [skillToDelete, setSkillToDelete] = useState<SkillMetadata | null>(
		null,
	);
	const { rememberTrigger, restoreFocus, setPrimaryFallback } =
		useDialogFocusReturn();

	const skillsQuery = useQuery(scope.list);
	const skills = skillsQuery.data ?? [];
	const editName = editor?.type === "edit" ? editor.name : "";
	const skillQuery = useQuery({
		...scope.detail(editName),
		enabled: Boolean(editName),
	});
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

	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen || isSubmitting) {
			return;
		}
		setEditor(null);
		onOpenChange(false);
	};

	const renderEditor = (view: EditorView) => {
		const isCreate = view.type === "create";
		let initialValues: SkillFormValues | undefined = emptySkillFormValues;
		let loadError: unknown;
		if (view.type === "edit") {
			initialValues = undefined;
			loadError = skillQuery.error;
			if (skillQuery.data) {
				const parsed = tryParseSkillMarkdown(skillQuery.data.content);
				if (parsed.ok) {
					initialValues = {
						name: skillQuery.data.name,
						description: skillQuery.data.description,
						body: parsed.values.body,
					};
				} else {
					loadError = new Error(parsed.error);
				}
			}
		}
		const title = isCreate
			? "New skill"
			: canEdit
				? "Edit skill"
				: "View skill";
		const header = (
			<DialogHeader className="px-6 pt-6">
				<DialogTitle>{title}</DialogTitle>
				<DialogDescription>
					Every chat in this project can use this skill.
				</DialogDescription>
			</DialogHeader>
		);
		const backButton = (disabled: boolean) => (
			<Button
				variant="outline"
				disabled={disabled}
				onClick={() => setEditor(null)}
			>
				Back
			</Button>
		);

		if (!initialValues) {
			return (
				<>
					{header}
					<div className="flex flex-col gap-4 px-6 py-4">
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
					<DialogFooter className="border-t border-border px-6 py-4">
						{backButton(false)}
					</DialogFooter>
				</>
			);
		}

		const mutation = isCreate ? createMutation : updateMutation;
		return (
			<SkillEditorForm
				mode={view.type}
				readOnly={!canEdit}
				initialValues={initialValues}
				existingNames={skills.map((skill) =>
					skill.name.toLocaleLowerCase("en-US"),
				)}
				submitError={skillError(
					mutation.error,
					`Failed to ${isCreate ? "create" : "save"} ${NOUN}.`,
					NOUN,
				)}
				isSubmitting={mutation.isPending}
				nameHint="Lowercase letters, numbers, and hyphens."
				bodyPlaceholder="Describe when and how the agent should use this skill."
				header={header}
				footer={({ isBusy, canSubmit }) => (
					<DialogFooter className="border-t border-border px-6 py-4">
						{backButton(isBusy)}
						{canEdit && (
							<Button type="submit" disabled={!canSubmit}>
								<Spinner loading={mutation.isPending} />
								Save
							</Button>
						)}
					</DialogFooter>
				)}
				onSubmit={(_values, content) => {
					const onSuccess = () => {
						setEditor(null);
						toast.success(`Project skill ${isCreate ? "created" : "saved"}.`);
					};
					if (view.type === "create") {
						createMutation.mutate({ content }, { onSuccess });
					} else {
						updateMutation.mutate(
							{ name: view.name, req: { content } },
							{ onSuccess },
						);
					}
				}}
			/>
		);
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent className="flex max-h-[90vh] max-w-2xl flex-col gap-0 overflow-hidden p-0">
				{editor ? (
					renderEditor(editor)
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
									onClick={() => {
										createMutation.reset();
										setEditor({ type: "create" });
									}}
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
							{skillsQuery.isLoading ? (
								<Loader />
							) : skills.length === 0 ? (
								!skillsQuery.error && (
									<div className="rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-content-secondary">
										No skills added yet
									</div>
								)
							) : (
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
											<TableRow key={skill.id}>
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
														title={formatDateTime(
															skill.updated_at,
															DATE_FORMAT.FULL_DATETIME,
														)}
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
																onPointerDown={rememberTrigger}
																onKeyDown={rememberTrigger}
															>
																{downloadMutation.isPending &&
																downloadMutation.variables === skill.name ? (
																	<Spinner className="size-4" loading />
																) : (
																	<EllipsisVerticalIcon aria-hidden="true" />
																)}
															</Button>
														</DropdownMenuTrigger>
														<DropdownMenuContent align="end">
															<DropdownMenuItem
																onClick={() => {
																	updateMutation.reset();
																	setEditor({ type: "edit", name: skill.name });
																}}
															>
																{canEdit ? "Edit" : "View"}
															</DropdownMenuItem>
															<DropdownMenuItem
																onClick={() =>
																	downloadMutation.mutate(skill.name)
																}
															>
																Download
															</DropdownMenuItem>
															{canEdit && (
																<>
																	<DropdownMenuSeparator />
																	<DropdownMenuItem
																		className="text-content-destructive focus:text-content-destructive"
																		onClick={() => {
																			deleteMutation.reset();
																			setSkillToDelete(skill);
																		}}
																	>
																		Delete&hellip;
																	</DropdownMenuItem>
																</>
															)}
														</DropdownMenuContent>
													</DropdownMenu>
												</TableCell>
											</TableRow>
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
