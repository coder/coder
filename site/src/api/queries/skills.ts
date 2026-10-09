import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

export type SkillOwner =
	| { type: "user"; user: string }
	| { type: "organization"; organizationId: string };

export const skillsKey = (owner: SkillOwner) =>
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
	skills: TypesGen.SkillMetadata[],
	skill: TypesGen.SkillMetadata,
): TypesGen.SkillMetadata[] => {
	const withoutSkill = skills.filter(({ name }) => name !== skill.name);
	return sortSkillMetadata([...withoutSkill, skill]);
};

const skillList = (owner: SkillOwner) => ({
	queryKey: skillsKey(owner),
	queryFn: (): Promise<TypesGen.SkillMetadata[]> => skillsAPI(owner).list(),
});

export const userSkills = (user = "me") => skillList({ type: "user", user });

export const organizationSkills = (organizationId: string) =>
	skillList({ type: "organization", organizationId });

export const skill = (owner: SkillOwner, name: string) => ({
	queryKey: skillKey(owner, name),
	queryFn: (): Promise<TypesGen.Skill> => skillsAPI(owner).get(name),
});

export const organizationSkillACLKey = (organizationId: string, name: string) =>
	[...skillKey({ type: "organization", organizationId }, name), "acl"] as const;

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
		await queryClient.invalidateQueries({
			queryKey: organizationSkillACLKey(organizationId, name),
			exact: true,
		});
	},
});

export const createSkill = (queryClient: QueryClient, owner: SkillOwner) => ({
	mutationFn: (req: TypesGen.CreateSkillRequest) =>
		skillsAPI(owner).create(req),
	onSuccess: (skill: TypesGen.Skill) => {
		queryClient.setQueryData<TypesGen.SkillMetadata[]>(
			skillsKey(owner),
			(skills) =>
				skills ? upsertSkillMetadata(skills, toSkillMetadata(skill)) : skills,
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

type ToggleSkillEnabledArgs = {
	name: string;
	enabled: boolean;
};

const toggleSkillEnabledKey = (owner: SkillOwner) =>
	[...skillsKey(owner), "toggle-enabled"] as const;

export const toggleSkillEnabled = (
	queryClient: QueryClient,
	owner: SkillOwner,
) => {
	const update = updateSkill(queryClient, owner);
	return {
		mutationKey: toggleSkillEnabledKey(owner),
		mutationFn: ({ name, enabled }: ToggleSkillEnabledArgs) =>
			update.mutationFn({ name, req: { enabled } }),
		onSuccess: (
			skill: TypesGen.Skill,
			{ name, enabled }: ToggleSkillEnabledArgs,
		) => update.onSuccess(skill, { name, req: { enabled } }),
	};
};

/**
 * Reads the mutation cache, which marks every toggle pending as soon as it
 * starts; rendered mutation state lags a click and tracks only the latest.
 */
export const isSkillTogglePending = (
	queryClient: QueryClient,
	owner: SkillOwner,
	name: string,
) =>
	queryClient.isMutating({
		mutationKey: toggleSkillEnabledKey(owner),
		predicate: ({ state: { variables } }) =>
			typeof variables === "object" &&
			variables !== null &&
			"name" in variables &&
			variables.name === name,
	}) > 0;

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
