import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

export const userSkillsKey = (user = "me") => ["user-skills", user] as const;

const userSkillKey = (name: string, user = "me") =>
	[...userSkillsKey(user), name] as const;

export const userSkills = (user = "me") => ({
	queryKey: userSkillsKey(user),
	queryFn: (): Promise<TypesGen.SkillMetadata[]> =>
		API.experimental.getUserSkills(user),
});

export const userSkill = (name: string, user = "me") => ({
	queryKey: userSkillKey(name, user),
	queryFn: (): Promise<TypesGen.Skill> =>
		API.experimental.getUserSkillByName(user, name),
});

export const createUserSkill = (queryClient: QueryClient, user = "me") => ({
	mutationFn: (req: TypesGen.CreateSkillRequest) =>
		API.experimental.createUserSkill(user, req),
	onSuccess: () =>
		queryClient.invalidateQueries({
			queryKey: userSkillsKey(user),
			exact: true,
		}),
});

type UpdateUserSkillArgs = {
	name: string;
	req: TypesGen.UpdateSkillRequest;
};

export const updateUserSkill = (queryClient: QueryClient, user = "me") => ({
	mutationFn: ({ name, req }: UpdateUserSkillArgs) =>
		API.experimental.updateUserSkill(user, name, req),
	// Invalidation refetches only active queries, so write the PATCH response
	// to keep a cached detail no editor observes from showing old content.
	onSuccess: (skill: TypesGen.Skill, { name }: UpdateUserSkillArgs) => {
		queryClient.setQueryData(userSkillKey(name, user), skill);
	},
	onSettled: () =>
		queryClient.invalidateQueries({
			queryKey: userSkillsKey(user),
			exact: true,
		}),
});

export const deleteUserSkill = (queryClient: QueryClient, user = "me") => ({
	mutationFn: (name: string) => API.experimental.deleteUserSkill(user, name),
	onSuccess: (_data: unknown, name: string) => {
		queryClient.removeQueries({
			queryKey: userSkillKey(name, user),
			exact: true,
		});
		return queryClient.invalidateQueries({
			queryKey: userSkillsKey(user),
			exact: true,
		});
	},
});
