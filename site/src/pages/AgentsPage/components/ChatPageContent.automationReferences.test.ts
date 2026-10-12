import { describe, expect, it } from "vitest";
import { toChatAutomationReferences } from "./ChatPageContent";

const loaded = new Map([
	["a1", { id: "a1", name: "CI heartbeat", kind: "schedule" as const }],
]);

describe("toChatAutomationReferences", () => {
	it.each([
		// A refetch keeps the loaded names: a missing one stays deleted.
		{
			query: { data: loaded, isError: false, isFetching: true },
			status: "settled",
		},
		{
			query: { data: loaded, isError: true, isFetching: false },
			status: "settled",
		},
		{
			query: { data: undefined, isError: false, isFetching: true },
			status: "loading",
		},
		{
			query: { data: undefined, isError: true, isFetching: false },
			status: "error",
		},
		{
			query: { data: undefined, isError: false, isFetching: false },
			status: "settled",
		},
	])("is $status for $query", ({ query, status }) => {
		expect(toChatAutomationReferences(query).status).toBe(status);
	});
});
