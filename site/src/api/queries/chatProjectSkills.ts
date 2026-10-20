import { isAxiosError } from "axios";
import type { UseQueryResult } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

/**
 * The key holds only the project ID, so it matches chatProjectSkills() for
 * any organization ID.
 */
export const chatProjectSkillsKey = (projectId: string) =>
	["chat-project-skills", projectId] as const;

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
 * runs, so a role grant does not count. A cached list can predate a share, so
 * a missing project counts once the list was fetched since mount, or is not
 * being refetched and its last refetch succeeded.
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
	if (projectQuery.data === undefined) {
		return "unknown";
	}
	if (projectQuery.data !== null) {
		return "usable";
	}
	const isListCurrent =
		projectQuery.isFetchedAfterMount ||
		(!projectQuery.isFetching && !projectQuery.error);
	return isListCurrent ? "unusable" : "unknown";
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
