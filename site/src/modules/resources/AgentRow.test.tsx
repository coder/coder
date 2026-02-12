import {
	MockWorkspaceApp,
	MockWorkspaceConnectionDERP,
	MockWorkspaceConnectionNoTelemetry,
	MockWorkspaceConnectionP2P,
} from "testHelpers/entities";
import { organizeAgentApps } from "./AgentApps/AgentApps";
import { connectionLabel, connectionTelemetrySummary } from "./AgentRow";

describe("organizeAgentApps", () => {
	test("returns one ungrouped app", () => {
		const result = organizeAgentApps([{ ...MockWorkspaceApp }]);

		expect(result).toEqual([{ apps: [MockWorkspaceApp] }]);
	});

	test("handles ordering correctly", () => {
		const bugApp = { ...MockWorkspaceApp, slug: "bug", group: "creatures" };
		const birdApp = { ...MockWorkspaceApp, slug: "bird", group: "creatures" };
		const fishApp = { ...MockWorkspaceApp, slug: "fish", group: "creatures" };
		const riderApp = { ...MockWorkspaceApp, slug: "rider" };
		const zedApp = { ...MockWorkspaceApp, slug: "zed" };
		const result = organizeAgentApps([
			bugApp,
			riderApp,
			birdApp,
			zedApp,
			fishApp,
		]);

		expect(result).toEqual([
			{ group: "creatures", apps: [bugApp, birdApp, fishApp] },
			{ apps: [riderApp] },
			{ apps: [zedApp] },
		]);
	});
});

describe("connectionLabel", () => {
	test("trims CLI ssh down to CLI for ssh connections", () => {
		expect(
			connectionLabel({
				...MockWorkspaceConnectionP2P,
				short_description: "CLI ssh",
				type: "ssh",
			}),
		).toBe("CLI · SSH");
	});

	test("avoids duplicating protocol when description already includes it", () => {
		expect(
			connectionLabel({
				...MockWorkspaceConnectionP2P,
				short_description: "SSH",
				type: "ssh",
			}),
		).toBe("SSH");
	});
});

describe("connectionTelemetrySummary", () => {
	test("returns direct summary with latency", () => {
		expect(connectionTelemetrySummary(MockWorkspaceConnectionP2P)).toBe(
			"0ms (Direct)",
		);
	});

	test("returns Relay summary with DERP name and latency", () => {
		expect(connectionTelemetrySummary(MockWorkspaceConnectionDERP)).toBe(
			"Relay via Frankfurt 45ms",
		);
	});

	test("returns null when no telemetry fields present", () => {
		expect(
			connectionTelemetrySummary(MockWorkspaceConnectionNoTelemetry),
		).toBeNull();
	});

	test("returns direct without latency when latency_ms is undefined", () => {
		expect(
			connectionTelemetrySummary({
				...MockWorkspaceConnectionP2P,
				latency_ms: undefined,
			}),
		).toBe("Direct");
	});

	test("returns Relay without region when home_derp is undefined", () => {
		expect(
			connectionTelemetrySummary({
				...MockWorkspaceConnectionDERP,
				home_derp: undefined,
			}),
		).toBe("Relay 45ms");
	});
});
