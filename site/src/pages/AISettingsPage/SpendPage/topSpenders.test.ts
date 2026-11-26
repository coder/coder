import { describe, expect, it } from "vitest";
import { MockOrganizationAISpendOverview } from "#/testHelpers/entities";
import { summarizeTopSpenders } from "./topSpenders";

const [mockUser] = MockOrganizationAISpendOverview.users;

describe("summarizeTopSpenders", () => {
	it("compares the listed users with everyone else", () => {
		expect(
			summarizeTopSpenders({
				count: 44,
				// $31,188.99 in total.
				totals: { spend_micros: 31_188_990_000, median_spend_micros: 0 },
				users: Array.from({ length: 10 }, () => ({
					...mockUser,
					// $2,849.60 each, $28,495.99 for the top 10.
					spend_micros: 2_849_599_000,
				})),
			}),
		).toBe(
			"Top 10 users spent 91% of the total. The other 34 users spent $2,693.00.",
		);
	});

	it("names a single listed and a single other user", () => {
		expect(
			summarizeTopSpenders({
				count: 2,
				// $4.00 in total, $2.00 median.
				totals: { spend_micros: 4_000_000, median_spend_micros: 2_000_000 },
				// $3.00.
				users: [{ ...mockUser, spend_micros: 3_000_000 }],
			}),
		).toBe("The top user spent 75% of the total. The other user spent $1.00.");
	});

	it.each([
		["every user is listed", MockOrganizationAISpendOverview],
		[
			"there is no spend",
			{
				count: 0,
				totals: { spend_micros: 0, median_spend_micros: 0 },
				users: [],
			},
		],
	])("returns nothing when %s", (_, overview) => {
		expect(summarizeTopSpenders(overview)).toBeUndefined();
	});
});
