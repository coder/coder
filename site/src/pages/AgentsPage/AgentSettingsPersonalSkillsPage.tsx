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
	| { type: "create" }
	| { type: "edit"; name: string }
	| { type: "delete"; skill: UserSkillMetadata }
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
		statusFallback =
			"You do not have permission to manage personal skills, or the skill limit was reached.";
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

const mutationToast = (error: unknown, fallback: string) => {
	const display = personalSkillError(error, fallback);
	if (!display) {
		return;
	}
	toast.error(display.message, { description: display.detail });
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
		onSuccess: async () => {
			await createMutationOptions.onSuccess?.();
			setDialogState(null);
			toast.success("Personal skill created.");
		},
		onError: (error) => {
			mutationToast(error, "Failed to create personal skill.");
		},
	});

	const updateMutationOptions = updateUserSkill(queryClient, user);
	const updateMutation = useMutation({
		...updateMutationOptions,
		onSuccess: async (skill, variables) => {
			await updateMutationOptions.onSuccess?.(skill, variables);
			setDialogState(null);
			toast.success("Personal skill saved.");
		},
		onError: (error) => {
			if (errorStatus(error) === 404) {
				setDialogState(null);
				void skillsQuery.refetch();
			}
			mutationToast(error, "Failed to save personal skill.");
		},
	});

	const deleteMutationOptions = deleteUserSkill(queryClient, user);
	const deleteMutation = useMutation({
		...deleteMutationOptions,
		onSuccess: async (data, variables) => {
			await deleteMutationOptions.onSuccess?.(data, variables);
			setDialogState(null);
			toast.success("Personal skill deleted.");
		},
		onError: (error) => {
			if (errorStatus(error) === 404) {
				setDialogState(null);
				void skillsQuery.refetch();
			}
			mutationToast(error, "Failed to delete personal skill.");
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
			submitError: personalSkillError(
				createMutation.error,
				"Failed to create personal skill.",
			),
			isSubmitting: createMutation.isPending,
			onSubmit: (_values, content) => {
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
			submitError: personalSkillError(
				updateMutation.error,
				"Failed to save personal skill.",
			),
			isSubmitting: updateMutation.isPending,
			onRetry: () => {
				void editSkillQuery.refetch();
			},
			onSubmit: (_values, content) => {
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
			error: personalSkillError(
				deleteMutation.error,
				"Failed to delete personal skill.",
			),
			isDeleting:
				deleteMutation.isPending &&
				deleteMutation.variables === dialogState.skill.name,
			onConfirm: () => {
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
