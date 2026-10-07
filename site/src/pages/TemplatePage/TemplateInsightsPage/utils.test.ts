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
});

describe("activeTimePercentage", () => {
	it("divides by the active time, not the sum of apps", () => {
		// Two apps open for the same hour each take the whole hour.
		expect(activeTimePercentage(3600, 3600)).toBe(100);
		expect(activeTimePercentage(900, 3600)).toBe(25);
	});

	it("caps at 100", () => {
		expect(activeTimePercentage(7200, 3600)).toBe(100);
	});

	it("returns 0 without active time", () => {
		expect(activeTimePercentage(60, 0)).toBe(0);
	});
});

describe("accountingRowDescription", () => {
	const reportLimit =
		"Time when apps past a single report's app limit were open. This row does not identify an individual app.";

	it.each<[string, Partial<TemplateAppUsage>, string | undefined]>([
		[
			"builtin unknown",
			{ slug: "unknown", family: "unknown" },
			"Usage reported without an app name, or with a reserved name. This row does not identify an individual app.",
		],
		["builtin overflow", { slug: "overflow", family: "unknown" }, reportLimit],
		[
			"builtin overflow with folded apps",
			{ slug: "overflow", family: "unknown", seconds_is_upper_bound: true },
			`${reportLimit} It also holds the unregistered apps past the 64 busiest. Which minutes those apps shared is not recorded, so this is the most time they could have been open.`,
		],
		["builtin app", {}, undefined],
		[
			"template app with an accounting slug",
			{ type: "app", slug: "overflow", family: "workspace_app" },
			undefined,
		],
	])("%s", (_, overrides, expected) => {
		expect(accountingRowDescription(usage(overrides))).toBe(expected);
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
