import { describe, expect, it } from "vitest";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { getWorkspaceStatusDisplay } from "./workspaceStatusDisplay";

describe("getWorkspaceStatusDisplay", () => {
	it("returns running status for a normal running workspace", () => {
		const workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				status: "running" as const,
			},
			health: { healthy: true, failing_agents: [] },
		};
		const agent = {
			...MockWorkspaceAgent,
			lifecycle_state: "ready" as const,
		};
		const result = getWorkspaceStatusDisplay(workspace, agent);
		expect(result.statusLabel).toBe("Workspace running");
		expect(result.statusIcon).toBeTruthy();
	});

	it("returns preparing status when agent is starting", () => {
		const workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				status: "running" as const,
			},
			health: { healthy: true, failing_agents: [] },
		};
		const agent = {
			...MockWorkspaceAgent,
			lifecycle_state: "starting" as const,
		};
		const result = getWorkspaceStatusDisplay(workspace, agent);
		expect(result.statusLabel).toBe("Workspace preparing");
		expect(result.statusIcon).toBeTruthy();
	});

	it("returns startup failed status when agent has start_error", () => {
		const workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				status: "running" as const,
			},
			health: { healthy: true, failing_agents: [] },
		};
		const agent = {
			...MockWorkspaceAgent,
			lifecycle_state: "start_error" as const,
		};
		const result = getWorkspaceStatusDisplay(workspace, agent);
		expect(result.statusLabel).toBe("Workspace startup failed");
		expect(result.statusIcon).toBeTruthy();
	});

	it("returns unhealthy label for an unhealthy workspace", () => {
		const workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				status: "running" as const,
			},
			health: {
				healthy: false,
				failing_agents: [MockWorkspaceAgent.id],
			},
		};
		const agent = {
			...MockWorkspaceAgent,
			lifecycle_state: "ready" as const,
		};
		const result = getWorkspaceStatusDisplay(workspace, agent);
		expect(result.statusLabel).toContain("(unhealthy)");
		expect(result.statusIcon).toBeTruthy();
	});
});
