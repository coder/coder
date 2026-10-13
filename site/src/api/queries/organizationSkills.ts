import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const organizationSkillsKey = (organizationId: string) =>
	["organization-skills", organizationId] as const;

const organizationSkillKey = (organizationId: string, name: string) =>
	[...organizationSkillsKey(organizationId), name] as const;

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
	skills: TypesGen.SkillMetadata[],
	skill: TypesGen.SkillMetadata,
): TypesGen.SkillMetadata[] => {
	const withoutSkill = skills.filter(({ name }) => name !== skill.name);
	return sortSkillMetadata([...withoutSkill, skill]);
};

export const organizationSkills = (organizationId: string) => ({
	queryKey: organizationSkillsKey(organizationId),
	queryFn: (): Promise<TypesGen.SkillMetadata[]> =>
		API.experimental.getOrganizationSkills(organizationId),
});

export const organizationSkill = (organizationId: string, name: string) => ({
	queryKey: organizationSkillKey(organizationId, name),
	queryFn: (): Promise<TypesGen.Skill> =>
		API.experimental.getOrganizationSkillByName(organizationId, name),
});

const organizationSkillACLKey = (organizationId: string, name: string) =>
	[...organizationSkillKey(organizationId, name), "acl"] as const;

export const organizationSkillACL = (organizationId: string, name: string) => ({
	queryKey: organizationSkillACLKey(organizationId, name),
	queryFn: (): Promise<TypesGen.OrganizationSkillACL> =>
		API.experimental.getOrganizationSkillACL(organizationId, name),
});

export const organizationSkillACLAvailable = (
	organizationId: string,
	name: string,
	options: TypesGen.UsersRequest,
) => ({
	queryKey: [
		...organizationSkillACLKey(organizationId, name),
		"available",
		options,
	] as const,
	queryFn: (): Promise<TypesGen.ACLAvailable> =>
		API.experimental.getOrganizationSkillACLAvailable(
			organizationId,
			name,
			options,
		),
});

type UpdateOrganizationSkillACLArgs = {
	organizationId: string;
	name: string;
	req: TypesGen.UpdateOrganizationSkillACLRequest;
};

export const updateOrganizationSkillACL = (queryClient: QueryClient) => ({
	mutationFn: ({ organizationId, name, req }: UpdateOrganizationSkillACLArgs) =>
		API.experimental.updateOrganizationSkillACL(organizationId, name, req),
	onSuccess: async (
		_data: unknown,
		{ organizationId, name }: UpdateOrganizationSkillACLArgs,
	) => {
		// Saving an ACL bumps the skill's updated_at, so its metadata is stale.
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: organizationSkillACLKey(organizationId, name),
				exact: true,
			}),
			queryClient.invalidateQueries({
				queryKey: organizationSkillsKey(organizationId),
				exact: true,
			}),
			queryClient.invalidateQueries({
				queryKey: organizationSkillKey(organizationId, name),
				exact: true,
			}),
		]);
	},
});

export const createOrganizationSkill = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: (req: TypesGen.CreateSkillRequest) =>
		API.experimental.createOrganizationSkill(organizationId, req),
	onSuccess: (skill: TypesGen.Skill) => {
		const skills = queryClient.getQueryData<TypesGen.SkillMetadata[]>(
			organizationSkillsKey(organizationId),
		);
		if (skills) {
			queryClient.setQueryData(
				organizationSkillsKey(organizationId),
				upsertSkillMetadata(skills, toSkillMetadata(skill)),
			);
		} else {
			void queryClient.invalidateQueries({
				queryKey: organizationSkillsKey(organizationId),
				exact: true,
			});
		}
		queryClient.setQueryData(
			organizationSkillKey(organizationId, skill.name),
			skill,
		);
	},
});

type UpdateOrganizationSkillArgs = {
	name: string;
	req: TypesGen.UpdateSkillRequest;
};

export const updateOrganizationSkill = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: ({ name, req }: UpdateOrganizationSkillArgs) =>
		API.experimental.updateOrganizationSkill(organizationId, name, req),
	onSuccess: (skill: TypesGen.Skill, { name }: UpdateOrganizationSkillArgs) => {
		queryClient.setQueryData(organizationSkillKey(organizationId, name), skill);
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			organizationSkillsKey(organizationId),
			(skills) =>
				skills ? upsertSkillMetadata(skills, toSkillMetadata(skill)) : skills,
		);
	},
});

export const deleteOrganizationSkill = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: (name: string) =>
		API.experimental.deleteOrganizationSkill(organizationId, name),
	onSuccess: (_data: unknown, name: string) => {
		// Not exact: the ACL queries are keyed under the skill, and a skill
		// recreated with the same name must not inherit them.
		queryClient.removeQueries({
			queryKey: organizationSkillKey(organizationId, name),
		});
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			organizationSkillsKey(organizationId),
			(skills) => skills?.filter((skill) => skill.name !== name),
		);
	},
});
