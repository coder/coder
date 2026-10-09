import { isAxiosError } from "axios";
import { saveAs } from "file-saver";
import JSZip from "jszip";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	createSkill,
	deleteSkill,
	isSkillTogglePending,
	organizationSkills,
	type SkillOwner,
	skill,
	toggleSkillEnabled,
	updateSkill,
	userSkills,
} from "#/api/queries/skills";
import type { SkillMetadata } from "#/api/typesGenerated";
import {
	parseSkillMarkdown,
	SKILLS_MAX_PER_OWNER,
	type SkillFormValues,
} from "../utils/skills";
import type { SkillErrorDisplay } from "./SkillEditor";
import {
	type SkillDeleteState,
	type SkillEditorState,
	type SkillsCopy,
	SkillsTableView,
	type SkillViewState,
} from "./SkillsTableView";

const emptySkillFormValues: SkillFormValues = {
	name: "",
	description: "",
	body: "",
};

type DialogState =
	| { type: "create"; submittedContent?: string }
	| { type: "edit"; name: string; submittedContent?: string }
	| { type: "view"; name: string }
	| { type: "delete"; skill: SkillMetadata; submittedName?: string }
	| null;

const skillError = (
	error: unknown,
	fallback: string,
	lowerNoun: string,
): SkillErrorDisplay | undefined => {
	if (!error) {
		return undefined;
	}

	const status = isAxiosError(error) ? error.response?.status : undefined;
	let statusFallback = fallback;
	if (status === 400) {
		statusFallback = "Skill content is invalid.";
	} else if (status === 403) {
		statusFallback = `You do not have permission to manage ${lowerNoun}s.`;
	} else if (status === 404) {
		statusFallback = `That ${lowerNoun} was not found.`;
	} else if (status === 409) {
		statusFallback = "A skill with that name already exists.";
	}

	return {
		message: getErrorMessage(error, statusFallback),
		detail: getErrorDetail(error),
	};
};

const downloadSkillFile = async (
	name: string,
	fetchContent: (name: string) => Promise<string>,
): Promise<void> => {
	const content = await fetchContent(name);
	saveAs(
		new Blob([content], { type: "text/markdown;charset=utf-8" }),
		`${name}.md`,
	);
};

const exportSkillsArchive = async (
	skills: readonly SkillMetadata[],
	fetchContent: (name: string) => Promise<string>,
	archiveName: string,
): Promise<void> => {
	const contents = await Promise.all(
		skills.map(async (skill) => ({
			name: skill.name,
			content: await fetchContent(skill.name),
		})),
	);
	const zip = new JSZip();
	for (const { name, content } of contents) {
		zip.file(`${name}/SKILL.md`, content);
	}
	const archive = await zip.generateAsync({ type: "blob" });
	saveAs(archive, archiveName);
};

type SkillsTableProps = {
	owner: SkillOwner;
	copy: SkillsCopy;
	canEdit: boolean;
};

export const SkillsTable: React.FC<SkillsTableProps> = ({
	owner,
	copy,
	canEdit,
}) => {
	const lowerNoun = copy.noun.toLocaleLowerCase("en-US");
	const queryClient = useQueryClient();
	const [dialogState, setDialogState] = useState<DialogState>(null);
	const skillsQuery = useQuery(
		owner.type === "user"
			? userSkills(owner.user)
			: organizationSkills(owner.organizationId),
	);
	const skills = skillsQuery.data ?? [];
	const existingNames = skills.map((skill) =>
		skill.name.toLocaleLowerCase("en-US"),
	);
	const detailName =
		dialogState?.type === "edit" || dialogState?.type === "view"
			? dialogState.name
			: "";
	const detailQuery = useQuery({
		...skill(owner, detailName),
		enabled: Boolean(detailName),
	});

	const createMutationOptions = createSkill(queryClient, owner);
	const createMutation = useMutation({
		...createMutationOptions,
		onSuccess: async (created, variables) => {
			await createMutationOptions.onSuccess(created);
			setDialogState((current) =>
				current?.type === "create" &&
				current.submittedContent === variables.content
					? null
					: current,
			);
			toast.success(`${copy.noun} created.`);
		},
	});

	const updateMutationOptions = updateSkill(queryClient, owner);
	const updateMutation = useMutation({
		...updateMutationOptions,
		onSuccess: async (updated, variables) => {
			await updateMutationOptions.onSuccess(updated, variables);
			setDialogState((current) =>
				current?.type === "edit" &&
				current.name === variables.name &&
				current.submittedContent === variables.req.content
					? null
					: current,
			);
			toast.success(`${copy.noun} saved.`);
		},
		onError: (error, variables) => {
			if (isAxiosError(error) && error.response?.status === 404) {
				toast.info(`That ${lowerNoun} was deleted while you were editing it.`);
				setDialogState((current) =>
					current?.type === "edit" &&
					current.name === variables.name &&
					current.submittedContent === variables.req.content
						? null
						: current,
				);
				void skillsQuery.refetch();
			}
		},
	});

	const toggleMutation = useMutation({
		...toggleSkillEnabled(queryClient, owner),
		onError: (error) => {
			void skillsQuery.refetch();
			if (isAxiosError(error) && error.response?.status === 404) {
				toast.info(
					`That ${lowerNoun} was deleted before your change was saved.`,
				);
				return;
			}
			toast.error(getErrorMessage(error, `Failed to update ${lowerNoun}.`), {
				description: getErrorDetail(error),
			});
		},
	});

	const deleteMutationOptions = deleteSkill(queryClient, owner);
	const deleteMutation = useMutation({
		...deleteMutationOptions,
		onSuccess: async (data, variables) => {
			await deleteMutationOptions.onSuccess(data, variables);
			setDialogState((current) =>
				current?.type === "delete" &&
				current.skill.name === variables &&
				current.submittedName === variables
					? null
					: current,
			);
			toast.success(`${copy.noun} deleted.`);
		},
		onError: (error, variables) => {
			if (isAxiosError(error) && error.response?.status === 404) {
				setDialogState((current) =>
					current?.type === "delete" &&
					current.skill.name === variables &&
					current.submittedName === variables
						? null
						: current,
				);
				void skillsQuery.refetch();
			}
		},
	});

	const fetchSkillContent = (name: string): Promise<string> =>
		queryClient.fetchQuery(skill(owner, name)).then((detail) => detail.content);

	const downloadMutation = useMutation({
		mutationFn: (name: string) => downloadSkillFile(name, fetchSkillContent),
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to download ${lowerNoun}.`), {
				description: getErrorDetail(error),
			});
		},
	});

	const exportAllMutation = useMutation({
		mutationFn: () =>
			exportSkillsArchive(skills, fetchSkillContent, copy.archiveName),
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to export ${lowerNoun}s.`), {
				description: getErrorDetail(error),
			});
		},
	});

	const downloadingSkillName = downloadMutation.isPending
		? downloadMutation.variables
		: undefined;
	const togglingSkill = toggleMutation.isPending
		? toggleMutation.variables
		: undefined;

	let editInitialValues: SkillFormValues | undefined;
	let editLoadError: unknown = detailQuery.error;
	if (dialogState?.type === "edit" && detailQuery.data) {
		try {
			const parsed = parseSkillMarkdown(detailQuery.data.content);
			editInitialValues = {
				name: detailQuery.data.name,
				description: detailQuery.data.description,
				body: parsed.body,
			};
		} catch (error) {
			editLoadError = error;
		}
	}

	let editorState: SkillEditorState | undefined;
	if (dialogState?.type === "create") {
		editorState = {
			mode: "create",
			initialValues: emptySkillFormValues,
			existingNames,
			submitError:
				createMutation.variables?.content === dialogState.submittedContent
					? skillError(
							createMutation.error,
							`Failed to create ${lowerNoun}.`,
							lowerNoun,
						)
					: undefined,
			isSubmitting: createMutation.isPending,
			onSubmit: (_values, content) => {
				setDialogState((current) =>
					current?.type === "create"
						? { ...current, submittedContent: content }
						: current,
				);
				createMutation.mutate({ content });
			},
			onClose: () => setDialogState(null),
		};
	} else if (dialogState?.type === "edit") {
		editorState = {
			mode: "edit",
			initialValues: editInitialValues,
			existingNames,
			loadError: editLoadError,
			isLoading: detailQuery.isLoading,
			isRetrying: detailQuery.isFetching,
			submitError:
				updateMutation.variables?.name === dialogState.name &&
				updateMutation.variables.req.content === dialogState.submittedContent
					? skillError(
							updateMutation.error,
							`Failed to save ${lowerNoun}.`,
							lowerNoun,
						)
					: undefined,
			isSubmitting: updateMutation.isPending,
			onRetry: () => {
				void detailQuery.refetch();
			},
			onSubmit: (_values, content) => {
				setDialogState((current) =>
					current?.type === "edit" && current.name === dialogState.name
						? { ...current, submittedContent: content }
						: current,
				);
				updateMutation.mutate({
					name: dialogState.name,
					req: { content },
				});
			},
			onClose: () => setDialogState(null),
		};
	}

	let viewState: SkillViewState | undefined;
	if (dialogState?.type === "view") {
		viewState = {
			name: dialogState.name,
			content: detailQuery.data?.content,
			loadError: detailQuery.error,
			isLoading: detailQuery.isLoading,
			isRetrying: detailQuery.isFetching,
			onRetry: () => {
				void detailQuery.refetch();
			},
			onClose: () => setDialogState(null),
		};
	}

	let deleteState: SkillDeleteState | undefined;
	if (dialogState?.type === "delete") {
		deleteState = {
			skill: dialogState.skill,
			error:
				deleteMutation.variables === dialogState.skill.name &&
				dialogState.submittedName === dialogState.skill.name
					? skillError(
							deleteMutation.error,
							`Failed to delete ${lowerNoun}.`,
							lowerNoun,
						)
					: undefined,
			isDeleting:
				deleteMutation.isPending &&
				deleteMutation.variables === dialogState.skill.name,
			onConfirm: () => {
				setDialogState((current) =>
					current?.type === "delete" &&
					current.skill.name === dialogState.skill.name
						? { ...current, submittedName: dialogState.skill.name }
						: current,
				);
				deleteMutation.mutate(dialogState.skill.name);
			},
			onClose: () => setDialogState(null),
		};
	}

	return (
		<SkillsTableView
			skills={skills}
			copy={copy}
			limit={SKILLS_MAX_PER_OWNER}
			canEdit={canEdit}
			error={skillsQuery.error}
			isLoading={skillsQuery.isLoading}
			isRetrying={skillsQuery.isFetching}
			onRetry={() => {
				void skillsQuery.refetch();
			}}
			onCreate={() => {
				if (skills.length >= SKILLS_MAX_PER_OWNER) {
					return;
				}
				createMutation.reset();
				setDialogState({ type: "create" });
			}}
			onEdit={(name) => {
				updateMutation.reset();
				setDialogState({ type: "edit", name });
			}}
			onView={(name) => {
				setDialogState({ type: "view", name });
			}}
			onDelete={(skill) => {
				deleteMutation.reset();
				setDialogState({ type: "delete", skill });
			}}
			onDownload={(skill) => {
				downloadMutation.mutate(skill.name);
			}}
			onExportAll={() => {
				exportAllMutation.mutate();
			}}
			onToggleEnabled={(skill, enabled) => {
				if (isSkillTogglePending(queryClient, owner, skill.name)) {
					return;
				}
				toggleMutation.mutate({ name: skill.name, enabled });
			}}
			downloadingSkillName={downloadingSkillName}
			togglingSkill={togglingSkill}
			isExportingAll={exportAllMutation.isPending}
			editorState={editorState}
			viewState={viewState}
			deleteState={deleteState}
		/>
	);
};
