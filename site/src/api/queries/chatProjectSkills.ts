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
