import { MutationObserver, QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { OrganizationSkillPreferences, Skill } from "#/api/typesGenerated";
import {
	MockOrganizationSkillPreferences,
	MockOrganizationSkillSettings,
} from "#/testHelpers/skills";
import {
	deleteOrganizationSkill,
	organizationSkill,
	organizationSkillACL,
	organizationSkillACLAvailable,
	organizationSkillPreferences,
	organizationSkillPreferencesKey,
	organizationSkillSettingsKey,
	organizationSkills,
	updateOrganizationSkillACL,
	updateOrganizationSkillPreferences,
	updateOrganizationSkillSettings,
} from "./organizationSkills";

afterEach(() => {
	vi.restoreAllMocks();
});

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

describe("organization skill settings queries", () => {
	it("stores the saved setting and invalidates member preferences", async () => {
		const queryClient = createTestQueryClient();
		const preferencesKey = organizationSkillPreferencesKey("org");
		queryClient.setQueryData(preferencesKey, MockOrganizationSkillPreferences);
		const mockSavedSettings = {
			...MockOrganizationSkillSettings,
			members_can_disable_skills: true,
		};

		await updateOrganizationSkillSettings(queryClient, "org").onSuccess(
			mockSavedSettings,
		);

		expect(
			queryClient.getQueryData(organizationSkillSettingsKey("org")),
		).toEqual(mockSavedSettings);
		expect(queryClient.getQueryState(preferencesKey)?.isInvalidated).toBe(true);
	});
});

describe("organization skill preference queries", () => {
	it("keeps a saved choice when an older read lands later", async () => {
		const queryClient = createTestQueryClient();
		const mockSavedPreferences: OrganizationSkillPreferences = {
			...MockOrganizationSkillPreferences,
			members_can_disable_skills: true,
			all_skills_disabled: true,
		};
		let resolveRead: (preferences: OrganizationSkillPreferences) => void =
			() => {};
		vi.spyOn(
			API.experimental,
			"getOrganizationSkillPreferences",
		).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					resolveRead = resolve;
				}),
		);
		const read = queryClient.fetchQuery(organizationSkillPreferences("org"));

		await updateOrganizationSkillPreferences(queryClient, "org").onSuccess(
			mockSavedPreferences,
		);
		resolveRead(MockOrganizationSkillPreferences);
		await expect(read).rejects.toThrow();

		expect(
			queryClient.getQueryData(organizationSkillPreferencesKey("org")),
		).toEqual(mockSavedPreferences);
	});

	it("applies one organization's writes in order", async () => {
		const queryClient = createTestQueryClient();
		const mockFirstPreferences: OrganizationSkillPreferences = {
			...MockOrganizationSkillPreferences,
			members_can_disable_skills: true,
			disabled_skill_ids: ["skill-a"],
		};
		const mockSecondPreferences: OrganizationSkillPreferences = {
			...mockFirstPreferences,
			disabled_skill_ids: ["skill-a", "skill-b"],
		};
		let resolveFirst: (preferences: OrganizationSkillPreferences) => void =
			() => {};
		const update = vi
			.spyOn(API.experimental, "updateOrganizationSkillPreferences")
			.mockImplementationOnce(
				() =>
					new Promise((resolve) => {
						resolveFirst = resolve;
					}),
			)
			.mockResolvedValueOnce(mockSecondPreferences);
		const options = updateOrganizationSkillPreferences(queryClient, "org");

		const first = new MutationObserver(queryClient, options).mutate({
			skills: { "skill-a": false },
		});
		const second = new MutationObserver(queryClient, options).mutate({
			skills: { "skill-b": false },
		});
		await vi.waitFor(() => expect(update).toHaveBeenCalled());
		resolveFirst(mockFirstPreferences);
		await Promise.all([first, second]);

		expect(
			queryClient.getQueryData(organizationSkillPreferencesKey("org")),
		).toEqual(mockSecondPreferences);
	});
});
