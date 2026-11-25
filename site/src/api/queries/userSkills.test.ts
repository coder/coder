import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type { Skill, SkillMetadata } from "#/api/typesGenerated";
import {
	createUserSkill,
	deleteUserSkill,
	updateUserSkill,
	userSkill,
	userSkills,
} from "./userSkills";

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

const makeSkill = (name: string, overrides: Partial<Skill> = {}): Skill => ({
	id: `${name}-id`,
	name,
	description: `${name} description`,
	enabled: true,
	action: false,
	model_invocable: true,
	content: `---\nname: ${name}\n---\nBody\n`,
	created_at: "2026-05-21T00:00:00Z",
	updated_at: "2026-05-21T00:00:00Z",
	...overrides,
});

const toMetadata = (skill: Skill): SkillMetadata => ({
	id: skill.id,
	name: skill.name,
	description: skill.description,
	enabled: skill.enabled,
	action: skill.action,
	model_invocable: skill.model_invocable,
	created_at: skill.created_at,
	updated_at: skill.updated_at,
});

describe("user skill queries", () => {
	it("defaults query keys to the current user alias", () => {
		expect(userSkills().queryKey).toEqual(["user-skills", "me"]);
		expect(userSkill("alpha").queryKey).toEqual(["user-skills", "me", "alpha"]);
		expect(userSkills("user-id").queryKey).toEqual(["user-skills", "user-id"]);
		expect(userSkill("alpha", "user-id").queryKey).toEqual([
			"user-skills",
			"user-id",
			"alpha",
		]);
	});

	it("invalidates the list after a create", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(userSkills().queryKey, []);

		await createUserSkill(queryClient).onSuccess();

		expect(
			queryClient.getQueryState(userSkills().queryKey)?.isInvalidated,
		).toBe(true);
	});

	it("writes the detail and invalidates the list after an update", async () => {
		const queryClient = createTestQueryClient();
		const alpha = makeSkill("alpha");
		const updatedAlpha = makeSkill("alpha", {
			description: "updated description",
			content:
				"---\nname: alpha\ndescription: updated description\n---\nUpdated\n",
			updated_at: "2026-05-21T01:00:00Z",
		});
		queryClient.setQueryData(userSkills().queryKey, [toMetadata(alpha)]);
		queryClient.setQueryData(userSkill("alpha").queryKey, alpha);

		const options = updateUserSkill(queryClient);
		options.onSuccess(updatedAlpha, {
			name: "alpha",
			req: { content: updatedAlpha.content },
		});
		await options.onSettled();

		expect(queryClient.getQueryData(userSkill("alpha").queryKey)).toEqual(
			updatedAlpha,
		);
		expect(
			queryClient.getQueryState(userSkills().queryKey)?.isInvalidated,
		).toBe(true);
	});

	it("removes a deleted skill's detail and invalidates the list", async () => {
		const queryClient = createTestQueryClient();
		const alpha = makeSkill("alpha");
		queryClient.setQueryData(userSkills().queryKey, [toMetadata(alpha)]);
		queryClient.setQueryData(userSkill("alpha").queryKey, alpha);

		await deleteUserSkill(queryClient).onSuccess(undefined, "alpha");

		expect(
			queryClient.getQueryData(userSkill("alpha").queryKey),
		).toBeUndefined();
		expect(
			queryClient.getQueryState(userSkills().queryKey)?.isInvalidated,
		).toBe(true);
	});
});
