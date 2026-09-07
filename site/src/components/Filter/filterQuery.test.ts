import { describe, expect, it } from "vitest";
import { parseFilterQuery, stringifyFilter } from "./filterQuery";

describe("stringifyFilter", () => {
	it("leaves simple values unquoted", () => {
		expect(stringifyFilter({ initiator: "me" })).toBe("initiator:me");
	});

	it("quotes values containing spaces", () => {
		expect(stringifyFilter({ session_id: "abc def" })).toBe(
			'session_id:"abc def"',
		);
	});

	it("quotes values containing colons so the backend parser accepts them", () => {
		expect(stringifyFilter({ started_after: "2026-08-16T20:42:00Z" })).toBe(
			'started_after:"2026-08-16T20:42:00Z"',
		);
	});

	it("drops empty values", () => {
		expect(stringifyFilter({ initiator: undefined, model: "gpt-4" })).toBe(
			"model:gpt-4",
		);
	});
});

describe("parseFilterQuery", () => {
	it("round-trips quoted timestamp values", () => {
		const values = {
			initiator: "me",
			started_after: "2026-08-16T20:42:00Z",
			started_before: "2026-08-17T20:42:00Z",
		};
		expect(parseFilterQuery(stringifyFilter(values))).toEqual(values);
	});

	it("parses unquoted and quoted pairs", () => {
		expect(parseFilterQuery('initiator:me session_id:"abc def"')).toEqual({
			initiator: "me",
			session_id: "abc def",
		});
	});

	it("returns an empty object for an empty query", () => {
		expect(parseFilterQuery("")).toEqual({});
	});
});
