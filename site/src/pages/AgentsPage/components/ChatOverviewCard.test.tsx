import { mockChatCost } from "#/testHelpers/chatEntities";
import { getOverviewCost } from "./ChatOverviewCard";

describe("getOverviewCost", () => {
	const cost = mockChatCost("root", 1_250_000);

	it.each([
		{
			name: "AI Gateway is off",
			showCost: false,
			data: cost,
			isError: false,
			expected: undefined,
		},
		{
			name: "the request is pending",
			showCost: true,
			data: undefined,
			isError: false,
			expected: undefined,
		},
		{
			name: "the request failed",
			showCost: true,
			data: undefined,
			isError: true,
			expected: "unavailable",
		},
		{
			name: "a refetch failed",
			showCost: true,
			data: cost,
			isError: true,
			expected: cost,
		},
		{
			name: "the chat has no requests",
			showCost: true,
			data: { ...cost, request_count: 0 },
			isError: false,
			expected: undefined,
		},
	])(
		"returns $expected when $name",
		({ showCost, data, isError, expected }) => {
			expect(getOverviewCost(showCost, { data, isError })).toEqual(expected);
		},
	);
});
