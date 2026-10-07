import type { TemplateAppUsage } from "#/api/typesGenerated";
import {
	accountingRowDescription,
	activeTimePercentage,
	appUsageKey,
	upperBoundLabel,
} from "./utils";

const usage = (overrides: Partial<TemplateAppUsage>): TemplateAppUsage => ({
	template_ids: [],
	type: "builtin",
	display_name: "VS Code",
	slug: "vscode",
	icon: "/icon/code.svg",
	seconds: 60,
	times_used: 0,
	family: "vscode",
	...overrides,
});

describe("appUsageKey", () => {
	it("keeps a session app and a template app sharing a slug apart", () => {
		const keys = [
			usage({}),
			usage({
				type: "app",
				display_name: "VS Code Web",
				family: "workspace_app",
			}),
			// The API groups template apps by slug, display name, and icon.
			usage({ type: "app", display_name: "VS Code", family: "workspace_app" }),
			usage({
				type: "app",
				display_name: "VS Code",
				icon: "/icon/other.svg",
				family: "workspace_app",
			}),
		].map(appUsageKey);
		expect(new Set(keys).size).toBe(keys.length);
	});

	it("does not collide when a field contains the separator", () => {
		expect(
			appUsageKey(usage({ type: "app", slug: "a", display_name: '","b' })),
		).not.toBe(
			appUsageKey(usage({ type: "app", slug: 'a","', display_name: "b" })),
		);
	});
});

describe("activeTimePercentage", () => {
	it("divides by the active time, not the sum of apps", () => {
		// Two apps open for the same hour each take the whole hour.
		expect(activeTimePercentage(3600, 3600)).toBe(100);
		expect(activeTimePercentage(900, 3600)).toBe(25);
	});

	it("is zero without active time", () => {
		expect(activeTimePercentage(60, 0)).toBe(0);
	});

	it("never exceeds 100", () => {
		expect(activeTimePercentage(7200, 3600)).toBe(100);
	});
});

describe("accountingRowDescription", () => {
	it("explains only the builtin accounting rows", () => {
		expect(
			accountingRowDescription(usage({ slug: "unknown", family: "unknown" })),
		).toMatch(/without an app name/);
		expect(
			accountingRowDescription(usage({ slug: "overflow", family: "unknown" })),
		).toMatch(/single report's app limit/);
		expect(accountingRowDescription(usage({}))).toBeUndefined();
		// A template app may use an accounting name as its slug.
		expect(
			accountingRowDescription(
				usage({ type: "app", slug: "overflow", family: "workspace_app" }),
			),
		).toBeUndefined();
	});

	it("says when folded apps make overflow an upper bound", () => {
		const exact = accountingRowDescription(
			usage({ slug: "overflow", family: "unknown" }),
		);
		const folded = accountingRowDescription(
			usage({
				slug: "overflow",
				family: "unknown",
				seconds_is_upper_bound: true,
			}),
		);
		expect(exact).not.toMatch(/busiest/);
		expect(folded).toMatch(/past the 64 busiest/);
	});
});

describe("upperBoundLabel", () => {
	it("prefixes only upper bounds", () => {
		expect(upperBoundLabel(usage({}), "2 hours")).toBe("2 hours");
		expect(
			upperBoundLabel(
				usage({ slug: "overflow", seconds_is_upper_bound: true }),
				"2 hours",
			),
		).toBe("Up to 2 hours");
	});
});
