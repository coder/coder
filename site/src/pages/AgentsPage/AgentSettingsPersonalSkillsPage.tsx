import { isAxiosError } from "axios";
import type { FC } from "react";
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
import type { UserSkillMetadata } from "#/api/typesGenerated";
import {
	AgentSettingsPersonalSkillsPageView,
	type PersonalSkillDeleteState,
	type PersonalSkillEditorState,
} from "./AgentSettingsPersonalSkillsPageView";
import type { PersonalSkillErrorDisplay } from "./components/PersonalSkillEditor";
import {
	PERSONAL_SKILLS_MAX_PER_USER,
	type PersonalSkillFormValues,
	parsePersonalSkillMarkdown,
} from "./utils/personalSkills";

const user = "me";

const emptySkillFormValues: PersonalSkillFormValues = {
	name: "",
	description: "",
	body: "",
};

type DialogState =
	| { type: "create"; submittedContent?: string }
	| { type: "edit"; name: string; submittedContent?: string }
	| { type: "delete"; skill: UserSkillMetadata; submittedName?: string }
	| null;

const errorStatus = (error: unknown): number | undefined =>
	isAxiosError(error) ? error.response?.status : undefined;

const personalSkillError = (
	error: unknown,
	fallback: string,
): PersonalSkillErrorDisplay | undefined => {
	if (!error) {
		return undefined;
	}

	const status = errorStatus(error);
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

const AgentSettingsPersonalSkillsPage: FC = () => {
	const queryClient = useQueryClient();
	const [dialogState, setDialogState] = useState<DialogState>(null);
	const skillsQuery = useQuery(userSkills(user));
	const skills = skillsQuery.data ?? [];
	const existingNames = skills.map((skill) =>
		skill.name.toLocaleLowerCase("en-US"),
	);
	const editName = dialogState?.type === "edit" ? dialogState.name : "";
	const editSkillQuery = useQuery({
		...userSkill(user, editName),
		enabled: Boolean(editName),
	});

	const createMutationOptions = createUserSkill(queryClient, user);
	const createMutation = useMutation({
		...createMutationOptions,
		onSuccess: async (_skill, variables) => {
			await createMutationOptions.onSuccess?.();
			setDialogState((current) =>
				current?.type === "create" &&
				current.submittedContent === variables.content
					? null
					: current,
			);
			toast.success("Personal skill created.");
		},
	});

	const updateMutationOptions = updateUserSkill(queryClient, user);
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
			if (errorStatus(error) === 404) {
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

	const deleteMutationOptions = deleteUserSkill(queryClient, user);
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
			if (errorStatus(error) === 404) {
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

	let editInitialValues: PersonalSkillFormValues | undefined;
	let editLoadError: unknown = editSkillQuery.error;
	if (editSkillQuery.data) {
		try {
			const parsed = parsePersonalSkillMarkdown(editSkillQuery.data.content);
			editInitialValues = {
				name: editSkillQuery.data.name,
				description: editSkillQuery.data.description,
				body: parsed.body,
			};
		} catch (error) {
			editLoadError = error;
		}
	}

	let editorState: PersonalSkillEditorState | undefined;
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

	let deleteState: PersonalSkillDeleteState | undefined;
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
		<AgentSettingsPersonalSkillsPageView
			skills={skills}
			error={skillsQuery.error}
			isLoading={skillsQuery.isLoading}
			isRetrying={skillsQuery.isFetching}
			onRetry={() => {
				void skillsQuery.refetch();
			}}
			onCreate={() => {
				if (skills.length >= PERSONAL_SKILLS_MAX_PER_USER) {
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
			editorState={editorState}
			deleteState={deleteState}
		/>
	);
};

export default AgentSettingsPersonalSkillsPage;
