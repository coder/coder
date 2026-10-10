import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

export type SkillOwner =
	| { type: "user"; user: string }
	| { type: "organization"; organizationId: string };

const skillsKey = (owner: SkillOwner) =>
	owner.type === "user"
		? (["user-skills", owner.user] as const)
		: (["organization-skills", owner.organizationId] as const);

const skillKey = (owner: SkillOwner, name: string) =>
	[...skillsKey(owner), name] as const;

const skillsAPI = (owner: SkillOwner) =>
	owner.type === "user"
		? {
				list: () => API.experimental.getUserSkills(owner.user),
				get: (name: string) =>
					API.experimental.getUserSkillByName(owner.user, name),
				create: (req: TypesGen.CreateSkillRequest) =>
					API.experimental.createUserSkill(owner.user, req),
				update: (name: string, req: TypesGen.UpdateSkillRequest) =>
					API.experimental.updateUserSkill(owner.user, name, req),
				delete: (name: string) =>
					API.experimental.deleteUserSkill(owner.user, name),
			}
		: {
				list: () =>
					API.experimental.getOrganizationSkills(owner.organizationId),
				get: (name: string) =>
					API.experimental.getOrganizationSkillByName(
						owner.organizationId,
						name,
					),
				create: (req: TypesGen.CreateSkillRequest) =>
					API.experimental.createOrganizationSkill(owner.organizationId, req),
				update: (name: string, req: TypesGen.UpdateSkillRequest) =>
					API.experimental.updateOrganizationSkill(
						owner.organizationId,
						name,
						req,
					),
				delete: (name: string) =>
					API.experimental.deleteOrganizationSkill(owner.organizationId, name),
			};

const toSkillMetadata = (skill: TypesGen.Skill): TypesGen.SkillMetadata => ({
	id: skill.id,
	name: skill.name,
	description: skill.description,
	enabled: skill.enabled,
	created_at: skill.created_at,
	updated_at: skill.updated_at,
});

const sortSkillMetadata = (
	skills: TypesGen.SkillMetadata[],
): TypesGen.SkillMetadata[] =>
	skills.toSorted((a, b) => a.name.localeCompare(b.name, "en-US"));

const upsertSkillMetadata = (
	skills: TypesGen.SkillMetadata[] | undefined,
	skill: TypesGen.SkillMetadata,
): TypesGen.SkillMetadata[] => {
	const withoutSkill = skills?.filter(({ name }) => name !== skill.name) ?? [];
	return sortSkillMetadata([...withoutSkill, skill]);
};

export const skillList = (owner: SkillOwner) => ({
	queryKey: skillsKey(owner),
	queryFn: (): Promise<TypesGen.SkillMetadata[]> => skillsAPI(owner).list(),
});

export const userSkills = (user = "me") => skillList({ type: "user", user });

export const skill = (owner: SkillOwner, name: string) => ({
	queryKey: skillKey(owner, name),
	queryFn: (): Promise<TypesGen.Skill> => skillsAPI(owner).get(name),
});

export const createSkill = (queryClient: QueryClient, owner: SkillOwner) => ({
	mutationFn: (req: TypesGen.CreateSkillRequest) =>
		skillsAPI(owner).create(req),
	onSuccess: (skill: TypesGen.Skill) => {
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			skillsKey(owner),
			(skills) => upsertSkillMetadata(skills, toSkillMetadata(skill)),
		);
		queryClient.setQueryData(skillKey(owner, skill.name), skill);
	},
});

type UpdateSkillArgs = {
	name: string;
	req: TypesGen.UpdateSkillRequest;
};

export const updateSkill = (queryClient: QueryClient, owner: SkillOwner) => ({
	mutationFn: ({ name, req }: UpdateSkillArgs) =>
		skillsAPI(owner).update(name, req),
	onSuccess: (skill: TypesGen.Skill, { name }: UpdateSkillArgs) => {
		queryClient.setQueryData(skillKey(owner, name), skill);
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			skillsKey(owner),
			(skills) =>
				skills ? upsertSkillMetadata(skills, toSkillMetadata(skill)) : skills,
		);
	},
});

export const deleteSkill = (queryClient: QueryClient, owner: SkillOwner) => ({
	mutationFn: (name: string) => skillsAPI(owner).delete(name),
	onSuccess: (_data: unknown, name: string) => {
		queryClient.removeQueries({
			queryKey: skillKey(owner, name),
			exact: true,
		});
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			skillsKey(owner),
			(skills) => skills?.filter((skill) => skill.name !== name),
		);
	},
});
