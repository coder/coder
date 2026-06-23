import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const userSkillsKey = (user = "me") => ["user-skills", user] as const;

const userSkillKey = (name: string, user = "me") =>
	[...userSkillsKey(user), name] as const;

const toUserSkillMetadata = (
	skill: TypesGen.UserSkill,
): TypesGen.UserSkillMetadata => ({
	id: skill.id,
	name: skill.name,
	description: skill.description,
	created_at: skill.created_at,
	updated_at: skill.updated_at,
});

const sortUserSkillMetadata = (
	skills: TypesGen.UserSkillMetadata[],
): TypesGen.UserSkillMetadata[] =>
	skills.toSorted((a, b) => a.name.localeCompare(b.name, "en-US"));

const upsertUserSkillMetadata = (
	skills: TypesGen.UserSkillMetadata[] | undefined,
	skill: TypesGen.UserSkillMetadata,
): TypesGen.UserSkillMetadata[] => {
	const withoutSkill = skills?.filter(({ name }) => name !== skill.name) ?? [];
	return sortUserSkillMetadata([...withoutSkill, skill]);
};

export const userSkills = (user = "me") => ({
	queryKey: userSkillsKey(user),
	queryFn: (): Promise<TypesGen.UserSkillMetadata[]> =>
		API.experimental.getUserSkills(user),
});

export const userSkill = (name: string, user = "me") => ({
	queryKey: userSkillKey(name, user),
	queryFn: (): Promise<TypesGen.UserSkill> =>
		API.experimental.getUserSkillByName(user, name),
});

export const createUserSkill = (queryClient: QueryClient, user = "me") => ({
	mutationFn: (req: TypesGen.CreateUserSkillRequest) =>
		API.experimental.createUserSkill(user, req),
	onSuccess: (skill: TypesGen.UserSkill) => {
		queryClient.setQueryData<TypesGen.UserSkillMetadata[]>(
			userSkillsKey(user),
			(skills) => upsertUserSkillMetadata(skills, toUserSkillMetadata(skill)),
		);
		queryClient.setQueryData(userSkillKey(skill.name, user), skill);
	},
});

type UpdateUserSkillArgs = {
	name: string;
	req: TypesGen.UpdateUserSkillRequest;
};

export const updateUserSkill = (queryClient: QueryClient, user = "me") => ({
	mutationFn: ({ name, req }: UpdateUserSkillArgs) =>
		API.experimental.updateUserSkill(user, name, req),
	onSuccess: (skill: TypesGen.UserSkill, { name }: UpdateUserSkillArgs) => {
		queryClient.setQueryData(userSkillKey(name, user), skill);
		queryClient.setQueryData<TypesGen.UserSkillMetadata[]>(
			userSkillsKey(user),
			(skills) =>
				skills
					? upsertUserSkillMetadata(skills, toUserSkillMetadata(skill))
					: skills,
		);
	},
});

export const deleteUserSkill = (queryClient: QueryClient, user = "me") => ({
	mutationFn: (name: string) => API.experimental.deleteUserSkill(user, name),
	onSuccess: (_data: unknown, name: string) => {
		queryClient.removeQueries({
			queryKey: userSkillKey(name, user),
			exact: true,
		});
		queryClient.setQueryData<TypesGen.UserSkillMetadata[]>(
			userSkillsKey(user),
			(skills) => skills?.filter((skill) => skill.name !== name),
		);
	},
});
