import {
	determineDefaultLtValue,
	filterByMaxTokenLifetime,
	type LifetimeDay,
	lifetimeDayPresets,
	NANO_DAY,
} from "./utils";

describe("unit/CreateTokenForm", () => {
	describe("filterByMaxTokenLifetime", () => {
		it.each<{
			maxTokenLifetime: number;
			expected: LifetimeDay[];
		}>([
			{ maxTokenLifetime: 6 * NANO_DAY, expected: [] },
			{
				maxTokenLifetime: 20 * NANO_DAY,
				expected: [lifetimeDayPresets[0]],
			},
			{
				maxTokenLifetime: 40 * NANO_DAY,
				expected: [lifetimeDayPresets[0], lifetimeDayPresets[1]],
			},
			{
				maxTokenLifetime: 70 * NANO_DAY,
				expected: [
					lifetimeDayPresets[0],
					lifetimeDayPresets[1],
					lifetimeDayPresets[2],
				],
			},
			{
				maxTokenLifetime: 100 * NANO_DAY,
				expected: lifetimeDayPresets,
			},
		])(
			"filterByMaxTokenLifetime($maxTokenLifetime)",
			({ maxTokenLifetime, expected }) => {
				expect(filterByMaxTokenLifetime(maxTokenLifetime)).toEqual(expected);
			},
		);
	});
	describe("determineDefaultLtValue", () => {
		it.each<{
			maxTokenLifetime: number;
			expected: string | number;
		}>([
			{
				maxTokenLifetime: 0,
				expected: 30,
			},
			{
				maxTokenLifetime: 60 * NANO_DAY,
				expected: 30,
			},
			{
				maxTokenLifetime: 20 * NANO_DAY,
				expected: 7,
			},
			{
				maxTokenLifetime: 2 * NANO_DAY,
				expected: "custom",
			},
		])(
			"determineDefaultLtValue($maxTokenLifetime)",
			({ maxTokenLifetime, expected }) => {
				expect(determineDefaultLtValue(maxTokenLifetime)).toEqual(expected);
			},
		);
	});
});
