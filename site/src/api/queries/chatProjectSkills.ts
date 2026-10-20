import { isAxiosError } from "axios";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const chatProjectSkillsKey = (projectId: string) =>
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
export const isChatProjectAccessError = (error: unknown): boolean => {
	const status = isAxiosError(error) ? error.response?.status : undefined;
	return status === 403 || status === 404;
};
