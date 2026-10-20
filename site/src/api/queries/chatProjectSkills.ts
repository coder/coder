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
});

/**
 * Reports whether a project skills request failed because the caller can no
 * longer read the project, for example after its share was removed. chatd
 * runs a turn without the skills of a project the chat owner cannot use, so
 * this failure means no project skills. Other failures may be transient and
 * leave the skills unknown.
 */
export const isChatProjectAccessError = (error: unknown): boolean => {
	const status = isAxiosError(error) ? error.response?.status : undefined;
	return status === 403 || status === 404;
};
