import { isAxiosError } from "axios";
import { saveAs } from "file-saver";
import JSZip from "jszip";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	createUserSkill,
	deleteUserSkill,
	updateUserSkill,
	userSkill,
	userSkills,
} from "#/api/queries/userSkills";
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
	SkillsTableView,
} from "./SkillsTableView";

const emptySkillFormValues: SkillFormValues = {
	name: "",
	description: "",
	body: "",
};

type DialogState =
	| { type: "create"; submittedContent?: string }
	| { type: "edit"; name: string; submittedContent?: string }
	| { type: "delete"; skill: SkillMetadata; submittedName?: string }
	| null;

const personalSkillError = (
	error: unknown,
	fallback: string,
): SkillErrorDisplay | undefined => {
	if (!error) {
		return undefined;
	}

	const status = isAxiosError(error) ? error.response?.status : undefined;
	let statusFallback = fallback;
	if (status === 400) {
		statusFallback = "Skill content is invalid.";
	} else if (status === 403) {
		statusFallback = "You do not have permission to manage personal skills.";
	} else if (status === 404) {
		statusFallback = "That personal skill was not found.";
	} else if (status === 409) {
		statusFallback = "A skill with that name already exists.";
	}

	return {
		message: getErrorMessage(error, statusFallback),
		detail: getErrorDetail(error),
	};
};

const downloadPersonalSkillFile = async (
	name: string,
	fetchContent: (name: string) => Promise<string>,
): Promise<void> => {
	const content = await fetchContent(name);
	saveAs(
		new Blob([content], { type: "text/markdown;charset=utf-8" }),
		`${name}.md`,
	);
};

const exportPersonalSkillsArchive = async (
	skills: readonly SkillMetadata[],
	fetchContent: (name: string) => Promise<string>,
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
	saveAs(archive, "personal-skills.zip");
};

const AgentSettingsPersonalSkillsPage: React.FC = () => {
	const queryClient = useQueryClient();
	const [dialogState, setDialogState] = useState<DialogState>(null);
	const skillsQuery = useQuery(userSkills());
	const skills = skillsQuery.data ?? [];
	const existingNames = skills.map((skill) =>
		skill.name.toLocaleLowerCase("en-US"),
	);
	const editName = dialogState?.type === "edit" ? dialogState.name : "";
	const editSkillQuery = useQuery({
		...userSkill(editName),
		enabled: Boolean(editName),
	});

	const createMutationOptions = createUserSkill(queryClient);
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
			toast.success("Personal skill created.");
		},
	});

	const updateMutationOptions = updateUserSkill(queryClient);
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
			toast.success("Personal skill saved.");
		},
		onError: (error, variables) => {
			if (isAxiosError(error) && error.response?.status === 404) {
				toast.info("That skill was deleted while you were editing it.");
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

	const deleteMutationOptions = deleteUserSkill(queryClient);
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
			toast.success("Personal skill deleted.");
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
		queryClient.fetchQuery(userSkill(name)).then((skill) => skill.content);

	const downloadMutation = useMutation({
		mutationFn: (name: string) =>
			downloadPersonalSkillFile(name, fetchSkillContent),
		onError: (error) => {
			toast.error(
				getErrorMessage(error, "Failed to download personal skill."),
				{
					description: getErrorDetail(error),
				},
			);
		},
	});

	const exportAllMutation = useMutation({
		mutationFn: () => exportPersonalSkillsArchive(skills, fetchSkillContent),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to export personal skills."), {
				description: getErrorDetail(error),
			});
		},
	});

	const downloadingSkillName = downloadMutation.isPending
		? downloadMutation.variables
		: undefined;

	let editInitialValues: SkillFormValues | undefined;
	let editLoadError: unknown = editSkillQuery.error;
	if (editSkillQuery.data) {
		try {
			const parsed = parseSkillMarkdown(editSkillQuery.data.content);
			editInitialValues = {
				name: editSkillQuery.data.name,
				description: editSkillQuery.data.description,
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
					? personalSkillError(
							createMutation.error,
							"Failed to create personal skill.",
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
			isLoading: editSkillQuery.isLoading,
			isRetrying: editSkillQuery.isFetching,
			submitError:
				updateMutation.variables?.name === dialogState.name &&
				updateMutation.variables.req.content === dialogState.submittedContent
					? personalSkillError(
							updateMutation.error,
							"Failed to save personal skill.",
						)
					: undefined,
			isSubmitting: updateMutation.isPending,
			onRetry: () => {
				void editSkillQuery.refetch();
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

	let deleteState: SkillDeleteState | undefined;
	if (dialogState?.type === "delete") {
		deleteState = {
			skill: dialogState.skill,
			error:
				deleteMutation.variables === dialogState.skill.name &&
				dialogState.submittedName === dialogState.skill.name
					? personalSkillError(
							deleteMutation.error,
							"Failed to delete personal skill.",
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

export default AgentSettingsPersonalSkillsPage;
