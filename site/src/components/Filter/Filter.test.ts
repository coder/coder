import { parseFilterQuery, stringifyFilter } from "./Filter";

describe("Filter query serialization", () => {
	it("parses hyphenated filter keys", () => {
		expect(parseFilterQuery("has-ai-seat:true status:active")).toEqual({
			"has-ai-seat": "true",
			status: "active",
		});
	});

	it("stringifies hyphenated filter keys", () => {
		expect(
			stringifyFilter({
				"has-ai-seat": "false",
				status: "suspended",
			}),
		).toBe("has-ai-seat:false status:suspended");
	});
});
