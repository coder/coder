import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type { Skill } from "#/api/typesGenerated";
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

const mockAlphaSkill: Skill = {
	id: "alpha-id",
	name: "alpha",
	description: "alpha description",
	enabled: true,
	action: false,
	model_invocable: true,
	content: "---\nname: alpha\n---\nBody\n",
	created_at: "2026-05-21T00:00:00Z",
	updated_at: "2026-05-21T00:00:00Z",
};

const { content: _content, ...mockAlphaSkillMetadata } = mockAlphaSkill;

describe("organization skill ACL queries", () => {
	it("removes a deleted skill's ACL caches", async () => {
		const queryClient = createTestQueryClient();
		const aclKey = organizationSkillACL("org", "alpha").queryKey;
		const availableKey = organizationSkillACLAvailable("org", "alpha", {
			q: "",
		}).queryKey;
		queryClient.setQueryData(organizationSkills("org").queryKey, [
			mockAlphaSkillMetadata,
		]);
		queryClient.setQueryData(aclKey, { users: [], groups: [] });
		queryClient.setQueryData(availableKey, { users: [], groups: [] });

		await deleteOrganizationSkill(queryClient, "org").onSuccess(
			undefined,
			"alpha",
		);

		expect(queryClient.getQueryData(aclKey)).toBeUndefined();
		expect(queryClient.getQueryData(availableKey)).toBeUndefined();
		expect(
			queryClient.getQueryState(organizationSkills("org").queryKey)
				?.isInvalidated,
		).toBe(true);
	});

	it("invalidates the skill list and detail after saving the ACL", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(organizationSkills("org").queryKey, [
			mockAlphaSkillMetadata,
		]);
		queryClient.setQueryData(
			organizationSkill("org", "alpha").queryKey,
			mockAlphaSkill,
		);

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
