import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type { Skill, SkillMetadata } from "#/api/typesGenerated";
import {
	createSkill,
	deleteSkill,
	type SkillOwner,
	skill,
	updateSkill,
	userSkills,
} from "./skills";

const me: SkillOwner = { type: "user", user: "me" };

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
	created_at: skill.created_at,
	updated_at: skill.updated_at,
});

describe("user skill queries", () => {
	it("defaults query keys to the current user alias", () => {
		expect(userSkills().queryKey).toEqual(["user-skills", "me"]);
		expect(skill(me, "alpha").queryKey).toEqual(["user-skills", "me", "alpha"]);
		expect(userSkills("user-id").queryKey).toEqual(["user-skills", "user-id"]);
		expect(skill({ type: "user", user: "user-id" }, "alpha").queryKey).toEqual([
			"user-skills",
			"user-id",
			"alpha",
		]);
	});

	it("adds a created skill to the sorted list cache", () => {
		const queryClient = createTestQueryClient();
		const alpha = makeSkill("alpha");
		const zeta = makeSkill("zeta");
		queryClient.setQueryData(userSkills().queryKey, [toMetadata(zeta)]);

		createSkill(queryClient, me).onSuccess(alpha);

		expect(queryClient.getQueryData(userSkills().queryKey)).toEqual([
			toMetadata(alpha),
			toMetadata(zeta),
		]);
		expect(queryClient.getQueryData(skill(me, "alpha").queryKey)).toEqual(
			alpha,
		);
	});

	it("updates list and detail caches for an updated skill", () => {
		const queryClient = createTestQueryClient();
		const alpha = makeSkill("alpha");
		const beta = makeSkill("beta");
		const updatedAlpha = makeSkill("alpha", {
			description: "updated description",
			content:
				"---\nname: alpha\ndescription: updated description\n---\nUpdated\n",
			updated_at: "2026-05-21T01:00:00Z",
		});
		queryClient.setQueryData(userSkills().queryKey, [
			toMetadata(alpha),
			toMetadata(beta),
		]);
		queryClient.setQueryData(skill(me, "alpha").queryKey, alpha);

		updateSkill(queryClient, me).onSuccess(updatedAlpha, {
			name: "alpha",
			req: { content: updatedAlpha.content },
		});

		expect(queryClient.getQueryData(userSkills().queryKey)).toEqual([
			toMetadata(updatedAlpha),
			toMetadata(beta),
		]);
		expect(queryClient.getQueryData(skill(me, "alpha").queryKey)).toEqual(
			updatedAlpha,
		);
	});

	it("removes a deleted skill from list and detail caches", () => {
		const queryClient = createTestQueryClient();
		const alpha = makeSkill("alpha");
		const beta = makeSkill("beta");
		queryClient.setQueryData(userSkills().queryKey, [
			toMetadata(alpha),
			toMetadata(beta),
		]);
		queryClient.setQueryData(skill(me, "alpha").queryKey, alpha);

		deleteSkill(queryClient, me).onSuccess(undefined, "alpha");

		expect(queryClient.getQueryData(userSkills().queryKey)).toEqual([
			toMetadata(beta),
		]);
		expect(
			queryClient.getQueryData(skill(me, "alpha").queryKey),
		).toBeUndefined();
	});
});
