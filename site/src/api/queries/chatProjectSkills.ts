import { isAxiosError } from "axios";
import type { QueryClient, UseQueryResult } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

/**
 * Keys a project's skill list. It holds only the project ID, so it matches
 * chatProjectSkills() for any organization ID. It prefixes each skill's
 * detail key, so invalidating it without `exact` also hits the details.
 */
export const chatProjectSkillsKey = (projectId: string) =>
	["chat-project-skills", projectId] as const;

/** Keys one project skill, under its project's list key. */
export const chatProjectSkillKey = (projectId: string, name: string) =>
	[...chatProjectSkillsKey(projectId), name] as const;

/** Lists the skills of a chat project, including disabled ones. */
export const chatProjectSkills = (
	organizationId: string,
	projectId: string,
) => ({
	queryKey: chatProjectSkillsKey(projectId),
	queryFn: (): Promise<TypesGen.SkillMetadata[]> =>
		API.experimental.getChatProjectSkills(organizationId, projectId),
	// An unknown list keeps /clear and /compact pending, so failures that
	// may be transient are retried.
	retry: (failureCount: number, error: unknown) =>
		failureCount < 3 && !isChatProjectAccessError(error),
});

/** Reads one project skill with its SKILL.md content. */
export const chatProjectSkill = (
	organizationId: string,
	projectId: string,
	name: string,
) => ({
	queryKey: chatProjectSkillKey(projectId, name),
	queryFn: (): Promise<TypesGen.Skill> =>
		API.experimental.getChatProjectSkill(organizationId, projectId, name),
});

const invalidateChatProjectSkills = (
	queryClient: QueryClient,
	projectId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatProjectSkillsKey(projectId),
		exact: true,
	});

/** Creates a project skill; every outcome, conflicts included, refreshes the list. */
export const createChatProjectSkill = (
	queryClient: QueryClient,
	organizationId: string,
	projectId: string,
) => ({
	mutationFn: (req: TypesGen.CreateSkillRequest) =>
		API.experimental.createChatProjectSkill(organizationId, projectId, req),
	// A 409 can mean a changed name set or skill count, so refresh the list.
	onSettled: () => invalidateChatProjectSkills(queryClient, projectId),
});

type UpdateChatProjectSkillArgs = {
	name: string;
	req: TypesGen.UpdateSkillRequest;
};

/** Replaces a project skill's content or turns it on or off. */
export const updateChatProjectSkill = (
	queryClient: QueryClient,
	organizationId: string,
	projectId: string,
) => ({
	mutationFn: ({ name, req }: UpdateChatProjectSkillArgs) =>
		API.experimental.updateChatProjectSkill(
			organizationId,
			projectId,
			name,
			req,
		),
	// Cancelling an older GET first keeps it from overwriting the PATCH response.
	onSuccess: async (
		skill: TypesGen.Skill,
		{ name }: UpdateChatProjectSkillArgs,
	) => {
		const queryKey = chatProjectSkillKey(projectId, name);
		await queryClient.cancelQueries({ queryKey, exact: true });
		queryClient.setQueryData(queryKey, skill);
	},
	onSettled: () => invalidateChatProjectSkills(queryClient, projectId),
});

/** Deletes a project skill. Success or a 404 evicts its details; the list always refreshes. */
export const deleteChatProjectSkill = (
	queryClient: QueryClient,
	organizationId: string,
	projectId: string,
) => ({
	mutationFn: (name: string) =>
		API.experimental.deleteChatProjectSkill(organizationId, projectId, name),
	onSuccess: (_data: unknown, name: string) => {
		queryClient.removeQueries({
			queryKey: chatProjectSkillKey(projectId, name),
			exact: true,
		});
	},
	onSettled: (_data: unknown, error: unknown, name: string) => {
		if (isAxiosError(error) && error.response?.status === 404) {
			queryClient.removeQueries({
				queryKey: chatProjectSkillKey(projectId, name),
				exact: true,
			});
		}
		return invalidateChatProjectSkills(queryClient, projectId);
	},
});

/**
 * Reports whether a project skills request failed with 403 or 404, meaning
 * the caller lost access to the project. Other failures may be transient.
 */
const isChatProjectAccessError = (error: unknown): boolean => {
	const status = isAxiosError(error) ? error.response?.status : undefined;
	return status === 403 || status === 404;
};

/** "unknown" means the caller's project list has not settled. */
type ChatProjectUsability = "usable" | "unusable" | "unknown";

/**
 * Classifies whether chatd loads a chat project's skills for the caller's
 * chats. chatProjects() holds only owned and shared projects, the check chatd
 * runs, so a role grant does not count. A cached list can predate a share or
 * its revocation, so the list counts once it was fetched since mount, or is
 * not being refetched and its last refetch succeeded.
 */
export const chatProjectUsability = (
	projectQuery: Pick<
		UseQueryResult<TypesGen.ChatProject | null, unknown>,
		"data" | "error" | "isFetching" | "isFetchedAfterMount"
	>,
	skillsError: unknown,
): ChatProjectUsability => {
	if (isChatProjectAccessError(skillsError)) {
		return "unusable";
	}
	const isListCurrent =
		projectQuery.isFetchedAfterMount ||
		(!projectQuery.isFetching && !projectQuery.error);
	if (projectQuery.data === undefined || !isListCurrent) {
		return "unknown";
	}
	return projectQuery.data === null ? "unusable" : "usable";
};

/**
 * Narrows a project's skills to what chatd loads. Undefined makes callers
 * treat the project like an unloaded skill source, which keeps triggers
 * qualified and built-in commands hidden.
 */
export const usableChatProjectSkills = <T>(
	usability: ChatProjectUsability,
	skills: T[] | undefined,
): T[] | undefined => {
	if (usability === "unknown") {
		return undefined;
	}
	return usability === "usable" ? skills : [];
};
