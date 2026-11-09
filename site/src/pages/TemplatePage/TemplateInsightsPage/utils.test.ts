import type { TemplateAppUsage } from "#/api/typesGenerated";
import {
	accountingRowDescription,
	activeTimePercentage,
	appUsageKey,
	upperBoundLabel,
} from "./utils";

const usage = (overrides: Partial<TemplateAppUsage>): TemplateAppUsage => ({
	template_ids: [],
	method: "ssh",
	display_name: "VS Code",
	slug: "vscode",
	icon: "/icon/code.svg",
	seconds: 60,
	times_used: 0,
	...overrides,
});

describe("appUsageKey", () => {
	it("keeps apps sharing a slug apart by method", () => {
		const keys = [
			usage({}),
			usage({ method: "workspace_app" }),
			usage({ method: "reconnecting_pty" }),
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
	const pastBusiest = "Includes apps past the 64 busiest.";

	it.each<[string, Partial<TemplateAppUsage>, string | undefined]>([
		[
			"session unknown",
			{ slug: "unknown" },
			"Includes all apps that did not report a name, or tried to use a reserved name.",
		],
		["session overflow", { slug: "overflow" }, pastBusiest],
		[
			"session overflow with folded apps",
			{ slug: "overflow", seconds_is_upper_bound: true },
			`${pastBusiest} Overlapping time isn't tracked, so this is the most time they could have been open.`,
		],
		["session app", {}, undefined],
		[
			"template app with an accounting slug",
			{ method: "workspace_app", slug: "overflow" },
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
