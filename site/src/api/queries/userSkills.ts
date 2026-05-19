import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const userSkillsKey = (user: string) => ["user-skills", user] as const;

const userSkillKey = (user: string, name: string) =>
	[...userSkillsKey(user), name] as const;

export const userSkills = (user: string) => ({
	queryKey: userSkillsKey(user),
	queryFn: (): Promise<TypesGen.UserSkillMetadata[]> =>
		API.experimental.getUserSkills(user),
});

export const userSkill = (user: string, name: string) => ({
	queryKey: userSkillKey(user, name),
	queryFn: (): Promise<TypesGen.UserSkill> =>
		API.experimental.getUserSkillByName(user, name),
});

export const createUserSkill = (queryClient: QueryClient, user: string) => ({
	mutationFn: (req: TypesGen.CreateUserSkillRequest) =>
		API.experimental.createUserSkill(user, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: userSkillsKey(user) });
	},
});

type UpdateUserSkillArgs = {
	name: string;
	req: TypesGen.UpdateUserSkillRequest;
};

export const updateUserSkill = (queryClient: QueryClient, user: string) => ({
	mutationFn: ({ name, req }: UpdateUserSkillArgs) =>
		API.experimental.updateUserSkill(user, name, req),
	onSuccess: async (
		_skill: TypesGen.UserSkill,
		{ name }: UpdateUserSkillArgs,
	) => {
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: userSkillsKey(user) }),
			queryClient.invalidateQueries({
				queryKey: userSkillKey(user, name),
				exact: true,
			}),
		]);
	},
});

export const deleteUserSkill = (queryClient: QueryClient, user: string) => ({
	mutationFn: (name: string) => API.experimental.deleteUserSkill(user, name),
	onSuccess: async (_data: unknown, name: string) => {
		queryClient.removeQueries({
			queryKey: userSkillKey(user, name),
			exact: true,
		});
		await queryClient.invalidateQueries({ queryKey: userSkillsKey(user) });
	},
});
