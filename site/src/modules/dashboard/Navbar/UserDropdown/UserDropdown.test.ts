import { describe, expect, it } from "vitest";
import type { UserAISpend } from "#/api/api";
import { toAISpend } from "./UserDropdown";

const spend = (overrides: Partial<UserAISpend> = {}): UserAISpend => ({
	user_id: "user-1",
	effective_group_id: null,
	limit_source: null,
	current_spend_micros: 0,
	period_start: "2024-01-01T00:00:00Z",
	period_end: "2024-02-01T00:00:00Z",
	spend_limit_micros: 1_000,
	...overrides,
});

describe("toAISpend", () => {
	it("returns null when not visible", () => {
		expect(toAISpend(false, spend())).toBeNull();
	});

	it("returns null when data hasn't loaded", () => {
		expect(toAISpend(true, undefined)).toBeNull();
	});

	it("returns null on negative current spend", () => {
		expect(toAISpend(true, spend({ current_spend_micros: -1 }))).toBeNull();
	});

	it("returns null on negative spend limit", () => {
		expect(toAISpend(true, spend({ spend_limit_micros: -1 }))).toBeNull();
	});

	it("shows an unlimited budget as 0% and normal severity", () => {
		expect(
			toAISpend(
				true,
				spend({ current_spend_micros: 500, spend_limit_micros: null }),
			),
		).toEqual({
			currentSpend: 500,
			spendLimit: null,
			percent: 0,
			severity: "normal",
		});
	});

	it("derives percent and severity from spend against the limit", () => {
		expect(
			toAISpend(
				true,
				spend({ current_spend_micros: 900, spend_limit_micros: 1_000 }),
			),
		).toEqual({
			currentSpend: 900,
			spendLimit: 1_000,
			percent: 90,
			severity: "warning",
		});
	});
});
