import { describe, expect, it } from "vitest";
import type {
	WorkspaceAgent,
	WorkspaceAgentLifecycle,
	WorkspaceAgentStatus,
} from "#/api/typesGenerated";
import { MockWorkspaceAgent } from "#/testHelpers/entities";
import { getAgentHealthIssues } from "./health";

interface AgentOverrides {
	status?: WorkspaceAgentStatus;
	lifecycle_state?: WorkspaceAgentLifecycle;
	parent_id?: string | null;
}

function buildAgent(overrides: AgentOverrides): WorkspaceAgent {
	return {
		...MockWorkspaceAgent,
		status: overrides.status ?? "connected",
		lifecycle_state: overrides.lifecycle_state ?? "ready",
		parent_id: overrides.parent_id ?? null,
	};
}

describe("getAgentHealthIssues", () => {
	it("returns disconnected issue for a disconnected agent", () => {
		expect(
			getAgentHealthIssues(buildAgent({ status: "disconnected" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Workspace agent has disconnected",
				severity: "warning",
				prominent: false,
			}),
		);
	});

	it("returns timeout issue for a timed-out agent", () => {
		expect(
			getAgentHealthIssues(buildAgent({ status: "timeout" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Agent is taking longer than expected to connect",
				severity: "warning",
				prominent: false,
			}),
		);
	});

	it("returns shutdown issue for shutdown lifecycle states", () => {
		expect(
			getAgentHealthIssues(buildAgent({ lifecycle_state: "shutting_down" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Workspace agent is shutting down",
				severity: "info",
			}),
		);
		expect(
			getAgentHealthIssues(buildAgent({ lifecycle_state: "shutdown_error" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Workspace agent is shutting down",
				severity: "info",
			}),
		);
		expect(
			getAgentHealthIssues(buildAgent({ lifecycle_state: "shutdown_timeout" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Workspace agent is shutting down",
				severity: "info",
			}),
		);
	});

	it("returns startup script issues", () => {
		expect(
			getAgentHealthIssues(buildAgent({ lifecycle_state: "start_error" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Startup script failed",
				severity: "warning",
				prominent: false,
			}),
		);
		expect(
			getAgentHealthIssues(buildAgent({ lifecycle_state: "start_timeout" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Startup script is taking longer than expected",
				severity: "warning",
				prominent: false,
			}),
		);
	});

	it("returns connecting issue for a connecting agent", () => {
		expect(
			getAgentHealthIssues(
				buildAgent({ status: "connecting", lifecycle_state: "starting" }),
			),
		).toContainEqual(
			expect.objectContaining({
				title: "Workspace agent is connecting",
				severity: "info",
				prominent: false,
			}),
		);
	});

	it("returns empty list for healthy ready connected agent", () => {
		expect(
			getAgentHealthIssues(
				buildAgent({ status: "connected", lifecycle_state: "ready" }),
			),
		).toEqual([]);
	});

	it("returns multiple issues when multiple conditions match", () => {
		const issues = getAgentHealthIssues(
			buildAgent({ status: "disconnected", lifecycle_state: "start_error" }),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({ title: "Workspace agent has disconnected" }),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({ title: "Startup script failed" }),
		);
	});
});
