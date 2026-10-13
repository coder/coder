import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type { Skill, SkillMetadata } from "#/api/typesGenerated";
import {
	deleteOrganizationSkill,
	organizationSkill,
	organizationSkillACL,
	organizationSkillACLAvailable,
	organizationSkills,
	updateOrganizationSkillACL,
} from "./organizationSkills";

const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: Number.POSITIVE_INFINITY,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

const makeSkill = (name: string): Skill => ({
	id: `${name}-id`,
	name,
	description: `${name} description`,
	enabled: true,
	content: `---\nname: ${name}\n---\nBody\n`,
	created_at: "2026-05-21T00:00:00Z",
	updated_at: "2026-05-21T00:00:00Z",
});

const toMetadata = (skill: Skill): SkillMetadata => ({
	id: skill.id,
	name: skill.name,
	description: skill.description,
	enabled: skill.enabled,
	created_at: skill.created_at,
	updated_at: skill.updated_at,
});

describe("organization skill ACL queries", () => {
	it("removes a deleted skill's ACL caches", () => {
		const queryClient = createTestQueryClient();
		const aclKey = organizationSkillACL("org", "alpha").queryKey;
		const availableKey = organizationSkillACLAvailable("org", "alpha", {
			q: "",
		}).queryKey;
		queryClient.setQueryData(organizationSkills("org").queryKey, [
			toMetadata(makeSkill("alpha")),
		]);
		queryClient.setQueryData(aclKey, { users: [], groups: [] });
		queryClient.setQueryData(availableKey, { users: [], groups: [] });

		deleteOrganizationSkill(queryClient, "org").onSuccess(undefined, "alpha");

		expect(queryClient.getQueryData(aclKey)).toBeUndefined();
		expect(queryClient.getQueryData(availableKey)).toBeUndefined();
		expect(
			queryClient.getQueryData(organizationSkills("org").queryKey),
		).toEqual([]);
	});

	it("invalidates the skill list and detail after saving the ACL", async () => {
		const queryClient = createTestQueryClient();
		const alpha = makeSkill("alpha");
		queryClient.setQueryData(organizationSkills("org").queryKey, [
			toMetadata(alpha),
		]);
		queryClient.setQueryData(organizationSkill("org", "alpha").queryKey, alpha);

		await updateOrganizationSkillACL(queryClient).onSuccess(undefined, {
			organizationId: "org",
			name: "alpha",
			req: {},
		});

		expect(
			queryClient.getQueryState(organizationSkills("org").queryKey)
				?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(organizationSkill("org", "alpha").queryKey)
				?.isInvalidated,
		).toBe(true);
	});
});
