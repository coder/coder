import { isAxiosError } from "axios";
import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const chatProjectSkillsKey = (projectId: string) =>
	["chat-project-skills", projectId] as const;

const chatProjectSkillKey = (projectId: string, name: string) =>
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
	// for a detail no editor observes, after cancelling an older GET.
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

/**
 * Deletes a project skill. Success, or a 404 for an already deleted skill,
 * evicts its cached details, and every outcome refreshes the list.
 */
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
