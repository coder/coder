import { describe, expect, it } from "vitest";
import { MockChatProject } from "#/testHelpers/entities";
import { MockSkill } from "#/testHelpers/skills";
import {
	chatProjectUsability,
	usableChatProjectSkills,
} from "./chatProjectSkills";

const httpError = (status: number) => ({
	isAxiosError: true,
	response: { status, data: { message: "Request failed." } },
});

const listQuery = {
	data: MockChatProject,
	error: null,
	isFetching: false,
	isFetchedAfterMount: true,
};

describe("chatProjectUsability", () => {
	it.each([
		{
			when: "the project list has the project",
			projectQuery: listQuery,
			skillsError: null,
			usability: "usable",
		},
		{
			when: "the project list fetched since mount lacks the project",
			projectQuery: { ...listQuery, data: null, isFetching: true },
			skillsError: null,
			usability: "unusable",
		},
		{
			when: "a cached project list that is not refetching lacks the project",
			projectQuery: { ...listQuery, data: null, isFetchedAfterMount: false },
			skillsError: null,
			usability: "unusable",
		},
		{
			when: "a cached project list being refetched lacks the project",
			projectQuery: {
				...listQuery,
				data: null,
				isFetching: true,
				isFetchedAfterMount: false,
			},
			skillsError: null,
			usability: "unknown",
		},
		{
			when: "the refetch of a cached project list failed",
			projectQuery: {
				...listQuery,
				data: null,
				error: httpError(500),
				isFetchedAfterMount: false,
			},
			skillsError: null,
			usability: "unknown",
		},
		{
			when: "a cached project list being refetched has the project",
			projectQuery: {
				...listQuery,
				isFetching: true,
				isFetchedAfterMount: false,
			},
			skillsError: null,
			usability: "unknown",
		},
		{
			when: "the refetch of a cached project list with the project failed",
			projectQuery: {
				...listQuery,
				error: httpError(500),
				isFetchedAfterMount: false,
			},
			skillsError: null,
			usability: "unknown",
		},
		{
			when: "a project list fetched since mount and refetching has the project",
			projectQuery: { ...listQuery, isFetching: true },
			skillsError: null,
			usability: "usable",
		},
		{
			when: "the project list is loading",
			projectQuery: {
				...listQuery,
				data: undefined,
				isFetching: true,
				isFetchedAfterMount: false,
			},
			skillsError: null,
			usability: "unknown",
		},
		{
			when: "the skills request fails with 404",
			projectQuery: { ...listQuery, data: undefined },
			skillsError: httpError(404),
			usability: "unusable",
		},
		{
			when: "the skills request fails with 500",
			projectQuery: listQuery,
			skillsError: httpError(500),
			usability: "usable",
		},
	])("is $usability when $when", ({ projectQuery, skillsError, usability }) => {
		expect(chatProjectUsability(projectQuery, skillsError)).toBe(usability);
	});
});

describe("usableChatProjectSkills", () => {
	it.each([
		{ usability: "usable", expected: [MockSkill] },
		{ usability: "unusable", expected: [] },
		{ usability: "unknown", expected: undefined },
	] as const)(
		"returns $expected when $usability",
		({ usability, expected }) => {
			expect(usableChatProjectSkills(usability, [MockSkill])).toEqual(expected);
		},
	);
});
