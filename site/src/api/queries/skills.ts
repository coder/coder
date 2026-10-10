import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const userSkillsKey = (user = "me") => ["user-skills", user] as const;

const userSkillKey = (name: string, user = "me") =>
	[...userSkillsKey(user), name] as const;

const toUserSkillMetadata = (
	skill: TypesGen.Skill,
): TypesGen.SkillMetadata => ({
	id: skill.id,
	name: skill.name,
	description: skill.description,
	enabled: skill.enabled,
	created_at: skill.created_at,
	updated_at: skill.updated_at,
});

const sortUserSkillMetadata = (
	skills: TypesGen.SkillMetadata[],
): TypesGen.SkillMetadata[] =>
	skills.toSorted((a, b) => a.name.localeCompare(b.name, "en-US"));

const upsertUserSkillMetadata = (
	skills: TypesGen.SkillMetadata[] | undefined,
	skill: TypesGen.SkillMetadata,
): TypesGen.SkillMetadata[] => {
	const withoutSkill = skills?.filter(({ name }) => name !== skill.name) ?? [];
	return sortUserSkillMetadata([...withoutSkill, skill]);
};

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
	onSuccess: (skill: TypesGen.Skill) => {
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			userSkillsKey(user),
			(skills) => upsertUserSkillMetadata(skills, toUserSkillMetadata(skill)),
		);
		queryClient.setQueryData(userSkillKey(skill.name, user), skill);
	},
});

type UpdateUserSkillArgs = {
	name: string;
	req: TypesGen.UpdateSkillRequest;
};

export const updateUserSkill = (queryClient: QueryClient, user = "me") => ({
	mutationFn: ({ name, req }: UpdateUserSkillArgs) =>
		API.experimental.updateUserSkill(user, name, req),
	onSuccess: (skill: TypesGen.Skill, { name }: UpdateUserSkillArgs) => {
		queryClient.setQueryData(userSkillKey(name, user), skill);
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
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
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			userSkillsKey(user),
			(skills) => skills?.filter((skill) => skill.name !== name),
		);
	},
});
