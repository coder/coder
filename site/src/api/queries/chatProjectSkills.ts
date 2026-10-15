import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

/** Keys the skill list of a chat project. */
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

/** Creates a project skill from SKILL.md content. */
export const createChatProjectSkill = (
	queryClient: QueryClient,
	organizationId: string,
	projectId: string,
) => ({
	mutationFn: (req: TypesGen.CreateSkillRequest) =>
		API.experimental.createChatProjectSkill(organizationId, projectId, req),
	// A 409 means the name was just taken, so refresh the list then too.
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
	// Invalidation refetches only active queries, so write the PATCH response
	// to keep a cached detail no editor observes from showing old content.
	onSuccess: (skill: TypesGen.Skill, { name }: UpdateChatProjectSkillArgs) => {
		queryClient.setQueryData(chatProjectSkillKey(projectId, name), skill);
	},
	onSettled: () => invalidateChatProjectSkills(queryClient, projectId),
});

/** Deletes a project skill by name. */
export const deleteChatProjectSkill = (
	queryClient: QueryClient,
	organizationId: string,
	projectId: string,
) => ({
	mutationFn: (name: string) =>
		API.experimental.deleteChatProjectSkill(organizationId, projectId, name),
	onSuccess: (_data: unknown, name: string) => {
		// A skill recreated with the same name must not show this content.
		queryClient.removeQueries({
			queryKey: chatProjectSkillKey(projectId, name),
			exact: true,
		});
	},
	// A 404 means someone else deleted it, so refresh the list then too.
	onSettled: () => invalidateChatProjectSkills(queryClient, projectId),
});
