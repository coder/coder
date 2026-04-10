import { describe, expect, it } from "vitest";
import type {
	WorkspaceAgent,
	WorkspaceAgentLifecycle,
	WorkspaceAgentStatus,
} from "#/api/typesGenerated";
import { MockWorkspaceAgent } from "#/testHelpers/entities";
import { getAgentHealthIssue } from "./health";

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

describe("getAgentHealthIssue", () => {
	it("returns disconnected issue for a disconnected agent", () => {
		expect(
			getAgentHealthIssue(buildAgent({ status: "disconnected" })),
		).toMatchObject({
			title: "Workspace agent has disconnected",
			severity: "warning",
			prominent: true,
		});
	});

	it("returns timeout issue for a timed-out agent", () => {
		expect(
			getAgentHealthIssue(buildAgent({ status: "timeout" })),
		).toMatchObject({
			title: "Agent is taking longer than expected to connect",
			severity: "warning",
			prominent: false,
		});
	});

	it("returns shutdown issue for shutdown lifecycle states", () => {
		expect(
			getAgentHealthIssue(buildAgent({ lifecycle_state: "shutting_down" })),
		).toMatchObject({
			title: "Workspace agent is shutting down",
			severity: "info",
		});
		expect(
			getAgentHealthIssue(buildAgent({ lifecycle_state: "shutdown_error" })),
		).toMatchObject({
			title: "Workspace agent is shutting down",
			severity: "info",
		});
		expect(
			getAgentHealthIssue(buildAgent({ lifecycle_state: "shutdown_timeout" })),
		).toMatchObject({
			title: "Workspace agent is shutting down",
			severity: "info",
		});
	});

	it("returns startup script issues", () => {
		expect(
			getAgentHealthIssue(buildAgent({ lifecycle_state: "start_error" })),
		).toMatchObject({
			title: "Startup script failed",
			severity: "warning",
			prominent: true,
		});
		expect(
			getAgentHealthIssue(buildAgent({ lifecycle_state: "start_timeout" })),
		).toMatchObject({
			title: "Startup script is taking longer than expected",
			severity: "warning",
			prominent: false,
		});
	});

	it("returns connecting issue for a connecting agent", () => {
		expect(
			getAgentHealthIssue(
				buildAgent({ status: "connecting", lifecycle_state: "starting" }),
			),
		).toMatchObject({
			title: "Workspace agent is connecting",
			severity: "info",
			prominent: false,
		});
	});

	it("returns undefined for healthy ready connected agent", () => {
		expect(
			getAgentHealthIssue(
				buildAgent({ status: "connected", lifecycle_state: "ready" }),
			),
		).toBeUndefined();
	});

	it("prioritizes status over lifecycle when both are set", () => {
		expect(
			getAgentHealthIssue(
				buildAgent({ status: "disconnected", lifecycle_state: "start_error" }),
			)?.title,
		).toBe("Workspace agent has disconnected");
		expect(
			getAgentHealthIssue(
				buildAgent({ status: "timeout", lifecycle_state: "shutting_down" }),
			)?.title,
		).toBe("Agent is taking longer than expected to connect");
	});
});
