import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceBuild,
	MockWorkspaceResource,
} from "testHelpers/entities";
import { describe, expect, it } from "vitest";
import type {
	Workspace,
	WorkspaceAgentLifecycle,
	WorkspaceAgentStatus,
} from "#/api/typesGenerated";
import { getAgentHealthIssue } from "./health";

interface AgentOverrides {
	status?: WorkspaceAgentStatus;
	lifecycle_state?: WorkspaceAgentLifecycle;
	parent_id?: string | null;
}

/**
 * Build a workspace mock with the given agent configurations and
 * failing-agent count. Defaults to status "connected" and lifecycle
 * "ready" so each test only needs to specify the fields it cares about.
 */
function buildWorkspace(
	agents: AgentOverrides[],
	failingAgentCount: number,
): Workspace {
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspaceBuild,
			resources: [
				{
					...MockWorkspaceResource,
					agents: agents.map((overrides, i) => ({
						...MockWorkspaceAgent,
						id: `agent-${i}`,
						name: `agent-${i}`,
						status: overrides.status ?? "connected",
						lifecycle_state: overrides.lifecycle_state ?? "ready",
						parent_id: overrides.parent_id ?? null,
					})),
				},
			],
		},
		health: {
			healthy: failingAgentCount === 0,
			failing_agents: Array.from(
				{ length: failingAgentCount },
				(_, i) => `agent-${i}`,
			),
		},
	};
}

describe("getAgentHealthIssue", () => {
	describe("individual branches", () => {
		it("returns disconnected issue for a disconnected agent", () => {
			const ws = buildWorkspace(
				[{ status: "disconnected", lifecycle_state: "ready" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Workspace agent has disconnected",
				detail:
					"Check the log output for errors. If agents do not reconnect, try restarting the workspace.",
				severity: "warning",
				prominent: true,
			});
		});

		it("returns timeout issue for a timed-out agent", () => {
			const ws = buildWorkspace(
				[{ status: "timeout", lifecycle_state: "ready" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Agent is taking longer than expected to connect",
				detail:
					"Continue to wait and check the log output for errors. If agents do not connect, try restarting the workspace.",
				severity: "warning",
				prominent: false,
			});
		});

		it("returns shutting down issue for shutting_down lifecycle", () => {
			const ws = buildWorkspace(
				[{ status: "connected", lifecycle_state: "shutting_down" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Workspace agent is shutting down",
				detail: "The workspace is not available while agents shut down.",
				severity: "info",
				prominent: false,
			});
		});

		it("returns shutting down issue for shutdown_error lifecycle", () => {
			const ws = buildWorkspace(
				[{ status: "connected", lifecycle_state: "shutdown_error" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Workspace agent is shutting down",
				detail: "The workspace is not available while agents shut down.",
				severity: "info",
				prominent: false,
			});
		});

		it("returns shutting down issue for shutdown_timeout lifecycle", () => {
			const ws = buildWorkspace(
				[{ status: "connected", lifecycle_state: "shutdown_timeout" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Workspace agent is shutting down",
				detail: "The workspace is not available while agents shut down.",
				severity: "info",
				prominent: false,
			});
		});

		it("returns start error issue for start_error lifecycle", () => {
			const ws = buildWorkspace(
				[{ status: "connected", lifecycle_state: "start_error" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Startup script failed",
				detail:
					"A startup script exited with an error. Check the agent logs for details.",
				severity: "warning",
				prominent: true,
			});
		});

		it("returns start timeout issue for start_timeout lifecycle", () => {
			const ws = buildWorkspace(
				[{ status: "connected", lifecycle_state: "start_timeout" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Startup script is taking longer than expected",
				detail:
					"A startup script has exceeded the expected time. Check the agent logs for details.",
				severity: "warning",
				prominent: false,
			});
		});

		it("returns connecting issue as default fallback", () => {
			const ws = buildWorkspace(
				[{ status: "connecting", lifecycle_state: "starting" }],
				1,
			);
			expect(getAgentHealthIssue(ws)).toEqual({
				title: "Workspace agent is still connecting",
				detail: "Check the log output if the connection does not complete.",
				severity: "info",
				prominent: false,
			});
		});
	});

	describe("plural path", () => {
		it("uses plural title when multiple agents are disconnected", () => {
			const ws = buildWorkspace(
				[
					{ status: "disconnected", lifecycle_state: "ready" },
					{ status: "disconnected", lifecycle_state: "ready" },
					{ status: "disconnected", lifecycle_state: "ready" },
				],
				3,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("3 workspace agents have disconnected");
		});

		it("uses plural title when multiple agents time out", () => {
			const ws = buildWorkspace(
				[
					{ status: "timeout", lifecycle_state: "ready" },
					{ status: "timeout", lifecycle_state: "ready" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe(
				"2 agents are taking longer than expected to connect",
			);
		});

		it("uses plural title when multiple agents have start errors", () => {
			const ws = buildWorkspace(
				[
					{ status: "connected", lifecycle_state: "start_error" },
					{ status: "connected", lifecycle_state: "start_error" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("Startup scripts failed on 2 agents");
		});

		it("uses plural title when multiple agents are shutting down", () => {
			const ws = buildWorkspace(
				[
					{ status: "connected", lifecycle_state: "shutting_down" },
					{ status: "connected", lifecycle_state: "shutdown_error" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("2 workspace agents are shutting down");
		});

		it("uses plural title when multiple agents are connecting", () => {
			const ws = buildWorkspace(
				[
					{ status: "connecting", lifecycle_state: "starting" },
					{ status: "connecting", lifecycle_state: "starting" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("2 workspace agents are still connecting");
		});

		it("uses singular title when only one agent is failing", () => {
			const ws = buildWorkspace(
				[{ status: "disconnected", lifecycle_state: "ready" }],
				1,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("Workspace agent has disconnected");
		});
	});

	describe("priority ordering", () => {
		it("disconnected takes priority over timeout", () => {
			const ws = buildWorkspace(
				[
					{ status: "disconnected", lifecycle_state: "ready" },
					{ status: "timeout", lifecycle_state: "ready" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("2 workspace agents have disconnected");
			expect(result.severity).toBe("warning");
			expect(result.prominent).toBe(true);
		});

		it("timeout takes priority over shutdown states", () => {
			const ws = buildWorkspace(
				[
					{ status: "timeout", lifecycle_state: "ready" },
					{ status: "connected", lifecycle_state: "shutting_down" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe(
				"2 agents are taking longer than expected to connect",
			);
			expect(result.severity).toBe("warning");
			expect(result.prominent).toBe(false);
		});

		it("shutdown states take priority over start_error", () => {
			const ws = buildWorkspace(
				[
					{ status: "connected", lifecycle_state: "shutting_down" },
					{ status: "connected", lifecycle_state: "start_error" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("2 workspace agents are shutting down");
			expect(result.severity).toBe("info");
		});

		it("start_error takes priority over start_timeout", () => {
			const ws = buildWorkspace(
				[
					{ status: "connected", lifecycle_state: "start_error" },
					{ status: "connected", lifecycle_state: "start_timeout" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("Startup scripts failed on 2 agents");
			expect(result.severity).toBe("warning");
			expect(result.prominent).toBe(true);
		});

		it("disconnected takes priority over all lifecycle states", () => {
			const ws = buildWorkspace(
				[
					{ status: "disconnected", lifecycle_state: "start_error" },
					{ status: "connected", lifecycle_state: "shutting_down" },
					{ status: "connected", lifecycle_state: "start_timeout" },
				],
				3,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("3 workspace agents have disconnected");
		});
	});

	describe("sub-agent filtering", () => {
		it("ignores a sub-agent whose status would change the result", () => {
			const ws = buildWorkspace(
				[
					// Parent agent: still connecting.
					{ status: "connecting", lifecycle_state: "starting" },
					// Sub-agent: disconnected, which would be highest priority
					// if not filtered out.
					{
						status: "disconnected",
						lifecycle_state: "ready",
						parent_id: "agent-0",
					},
				],
				1,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("Workspace agent is still connecting");
			expect(result.severity).toBe("info");
		});

		it("ignores a sub-agent whose lifecycle would promote severity", () => {
			const ws = buildWorkspace(
				[
					// Parent agent: soft start_timeout issue.
					{ status: "connected", lifecycle_state: "start_timeout" },
					// Sub-agent: start_error, which would take priority over
					// start_timeout if not filtered.
					{
						status: "connected",
						lifecycle_state: "start_error",
						parent_id: "agent-0",
					},
				],
				1,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe(
				"Startup script is taking longer than expected",
			);
			expect(result.prominent).toBe(false);
		});
	});

	describe("start_timeout reachability", () => {
		it("is overshadowed by start_error in a multi-agent workspace", () => {
			const ws = buildWorkspace(
				[
					{ status: "connected", lifecycle_state: "start_timeout" },
					{ status: "connected", lifecycle_state: "start_error" },
				],
				2,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe("Startup scripts failed on 2 agents");
			expect(result.prominent).toBe(true);
		});

		it("is returned when it is the sole lifecycle issue", () => {
			// In a multi-agent workspace another agent may have triggered
			// the unhealthy flag while this agent only has start_timeout.
			const ws = buildWorkspace(
				[
					{ status: "connected", lifecycle_state: "start_timeout" },
					{ status: "connected", lifecycle_state: "ready" },
				],
				1,
			);
			const result = getAgentHealthIssue(ws);
			expect(result.title).toBe(
				"Startup script is taking longer than expected",
			);
			expect(result.severity).toBe("warning");
			expect(result.prominent).toBe(false);
		});
	});
});
