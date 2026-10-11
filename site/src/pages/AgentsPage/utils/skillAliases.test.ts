import { describe, expect, it } from "vitest";
import { resolveSkillTriggers } from "./skillAliases";

const skills = (...names: string[]) =>
	names.map((name) => ({ name, description: `${name} description` }));

type Lists = {
	personal?: readonly string[];
	org?: readonly string[];
	workspace?: readonly string[];
};

const triggersFor = (lists: Lists) =>
	resolveSkillTriggers([
		{ source: "personal", skills: lists.personal && skills(...lists.personal) },
		{ source: "org", skills: lists.org && skills(...lists.org) },
		{
			source: "workspace",
			skills: lists.workspace && skills(...lists.workspace),
		},
	]).map((trigger) => trigger.triggerText);

describe("resolveSkillTriggers", () => {
	it.each<{ name: string; lists: Lists; expected: string[] }>([
		{
			name: "keeps names unique to one source bare, except workspace",
			lists: { personal: ["review"], org: ["deploy"], workspace: ["test"] },
			expected: ["/review", "/deploy", "/workspace/test"],
		},
		{
			name: "qualifies a personal and organization collision in both",
			lists: { personal: ["review"], org: ["review", "deploy"], workspace: [] },
			expected: ["/personal/review", "/org/review", "/deploy"],
		},
		{
			name: "qualifies an organization and workspace collision",
			lists: { personal: ["plan"], org: ["test"], workspace: ["test"] },
			expected: ["/plan", "/org/test", "/workspace/test"],
		},
		{
			name: "qualifies a name found in every source",
			lists: { personal: ["review"], org: ["review"], workspace: ["review"] },
			expected: ["/personal/review", "/org/review", "/workspace/review"],
		},
		{
			name: "keeps triggers qualified while a list is unknown",
			lists: { personal: ["review"], workspace: ["test"] },
			expected: ["/personal/review", "/workspace/test"],
		},
	])("$name", ({ lists, expected }) => {
		expect(triggersFor(lists)).toEqual(expected);
	});

	it("keeps the qualified alias searchable for bare triggers", () => {
		expect(
			resolveSkillTriggers([
				{ source: "org", skills: skills("deploy") },
				{ source: "personal", skills: [] },
			]),
		).toEqual([
			{
				source: "org",
				name: "deploy",
				description: "deploy description",
				triggerText: "/deploy",
				altTriggerText: "/org/deploy",
			},
		]);
	});
});
