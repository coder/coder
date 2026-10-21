import { isAxiosError } from "axios";
import { saveAs } from "file-saver";
import JSZip from "jszip";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { SkillMetadata } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import {
	emptySkillFormValues,
	SKILLS_MAX_PER_OWNER,
	type SkillAccess,
	type SkillsCopy,
	type SkillsScope,
	skillFormValuesFromSkill,
} from "../utils/skills";
import type { SectionHeaderLevel } from "./SectionHeader";
import type { SkillErrorDisplay } from "./SkillEditor";
import {
	type SkillDeleteState,
	type SkillEditorState,
	SkillsTableView,
	type SkillsTableViewProps,
} from "./SkillsTableView";

type DialogState =
	| { type: "create"; submittedContent?: string }
	| { type: "edit"; name: string; submittedContent?: string }
	| { type: "view"; name: string }
	| { type: "delete"; skill: SkillMetadata; submittedName?: string }
	| null;

/** Maps an API error to a status-specific message with a fallback; undefined without an error. */
export const skillError = (
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

/** Fetches a skill's SKILL.md content and saves it as `<name>.md`. */
export const downloadSkillFile = async (
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
	scope: SkillsScope;
	copy: SkillsCopy;
	access: SkillAccess;
	enabledOnly?: boolean;
	headerLevel?: SectionHeaderLevel;
	toolbar?: React.ReactNode;
	onManagePermissions?: SkillsTableViewProps["onManagePermissions"];
};

export const SkillsTable: React.FC<SkillsTableProps> = ({
	scope,
	copy,
	access,
	enabledOnly = false,
	headerLevel,
	toolbar,
	onManagePermissions,
}) => {
	const lowerNoun = copy.noun.toLocaleLowerCase("en-US");
	const queryClient = useQueryClient();
	const [dialogState, setDialogState] = useState<DialogState>(null);
	const skillsQuery = useQuery(scope.list);
	const skills = (skillsQuery.data ?? []).filter(
		(skill) => !enabledOnly || skill.enabled,
	);
	const existingNames = skills.map((skill) =>
		skill.name.toLocaleLowerCase("en-US"),
	);
	const editName =
		dialogState?.type === "edit" || dialogState?.type === "view"
			? dialogState.name
			: "";
	const editSkillQuery = useQuery({
		...scope.detail(editName),
		enabled: Boolean(editName),
	});

	const createMutationOptions = scope.create(queryClient);
	const createMutation = useMutation({
		...createMutationOptions,
		onSuccess: async (_skill, variables) => {
			await createMutationOptions.onSuccess?.(_skill);
			setDialogState((current) =>
				current?.type === "create" &&
				current.submittedContent === variables.content
					? null
					: current,
			);
			toast.success(`${copy.noun} created.`);
		},
	});

	const updateMutationOptions = scope.update(queryClient);
	const updateMutation = useMutation({
		...updateMutationOptions,
		onSuccess: async (skill, variables) => {
			await updateMutationOptions.onSuccess?.(skill, variables);
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

	const deleteMutationOptions = scope.remove(queryClient);
	const deleteMutation = useMutation({
		...deleteMutationOptions,
		onSuccess: async (data, variables) => {
			await deleteMutationOptions.onSuccess?.(data, variables);
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
		queryClient.fetchQuery(scope.detail(name)).then((detail) => detail.content);

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

	const editLoaded = editSkillQuery.data
		? skillFormValuesFromSkill(editSkillQuery.data)
		: undefined;
	const editInitialValues = editLoaded?.ok ? editLoaded.values : undefined;
	const editLoadError =
		editLoaded && !editLoaded.ok
			? new Error(editLoaded.error)
			: editSkillQuery.error;

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
			onSubmit: (_values, content, onSettled) => {
				setDialogState((current) =>
					current?.type === "create"
						? { ...current, submittedContent: content }
						: current,
				);
				createMutation.mutate({ content }, { onSettled });
			},
			onClose: () => setDialogState(null),
		};
	} else if (dialogState?.type === "edit" || dialogState?.type === "view") {
		const submittedContent =
			dialogState.type === "edit" ? dialogState.submittedContent : undefined;
		editorState = {
			mode: "edit",
			readOnly: dialogState.type === "view",
			initialValues: editInitialValues,
			existingNames,
			loadError: editLoadError,
			isLoading: editSkillQuery.isLoading,
			isRetrying: editSkillQuery.isFetching,
			submitError:
				updateMutation.variables?.name === dialogState.name &&
				updateMutation.variables.req.content === submittedContent
					? skillError(
							updateMutation.error,
							`Failed to save ${lowerNoun}.`,
							lowerNoun,
						)
					: undefined,
			isSubmitting: updateMutation.isPending,
			onRetry: () => {
				void editSkillQuery.refetch();
			},
			onSubmit: (_values, content, onSettled) => {
				setDialogState((current) =>
					current?.type === "edit" && current.name === dialogState.name
						? { ...current, submittedContent: content }
						: current,
				);
				updateMutation.mutate(
					{ name: dialogState.name, req: { content } },
					{ onSettled },
				);
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
			scope={scope}
			skills={skills}
			copy={copy}
			access={access}
			headerLevel={headerLevel}
			toolbar={toolbar}
			onManagePermissions={onManagePermissions}
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
				setDialogState({ type: access.update ? "edit" : "view", name });
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
			downloadingSkillName={downloadingSkillName}
			isExportingAll={exportAllMutation.isPending}
			editorState={editorState}
			deleteState={deleteState}
		/>
	);
};
