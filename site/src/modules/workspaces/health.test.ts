import { describe, expect, it } from "vitest";
import type {
	WorkspaceAgent,
	WorkspaceAgentLifecycle,
	WorkspaceAgentStatus,
} from "#/api/typesGenerated";
import {
	MockWorkspaceAgent,
	MockWorkspaceAgentStartError,
	MockWorkspaceAgentStartTimeout,
} from "#/testHelpers/entities";
import {
	getAgentConnectivityIssues,
	getAgentHealthIssues,
	getAgentScriptIssues,
} from "./health";

interface AgentOverrides {
	status?: WorkspaceAgentStatus;
	lifecycle_state?: WorkspaceAgentLifecycle;
	parent_id?: string | null;
}

function buildAgent(overrides: AgentOverrides): WorkspaceAgent {
	return {
		...MockWorkspaceAgent,
		...overrides,
	};
}

describe("getAgentConnectivityIssues", () => {
	it("returns disconnected issue for a disconnected agent", () => {
		expect(
			getAgentConnectivityIssues(buildAgent({ status: "disconnected" })),
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
			getAgentConnectivityIssues(buildAgent({ status: "timeout" })),
		).toContainEqual(
			expect.objectContaining({
				title: "Agent is taking longer than expected to connect",
				severity: "warning",
				prominent: false,
			}),
		);
	});

	it("does not return script issues", () => {
		const issues = getAgentConnectivityIssues(
			buildAgent(MockWorkspaceAgentStartError),
		);
		expect(issues).not.toContainEqual(
			expect.objectContaining({
				title: `"Startup Script" failed`,
			}),
		);
	});

	it("returns empty list for healthy ready connected agent", () => {
		expect(
			getAgentConnectivityIssues(
				buildAgent({ status: "connected", lifecycle_state: "ready" }),
			),
		).toEqual([]);
	});

	it("returns connecting issue for a connecting agent", () => {
		expect(
			getAgentConnectivityIssues(
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

	it("returns shutdown issue for shutdown lifecycle states", () => {
		for (const lifecycle_state of [
			"shutting_down",
			"shutdown_error",
			"shutdown_timeout",
		] as const) {
			expect(
				getAgentConnectivityIssues(buildAgent({ lifecycle_state })),
			).toContainEqual(
				expect.objectContaining({
					title: "Workspace agent is shutting down",
					severity: "info",
				}),
			);
		}
	});
});

describe("getAgentScriptIssues", () => {
	it("returns script issues", () => {
		const issues = getAgentScriptIssues(
			buildAgent(MockWorkspaceAgentStartError),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({
				title: `"Startup Script" failed`,
				severity: "warning",
				prominent: false,
			}),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({
				title: `"time" is taking longer than expected`,
				severity: "warning",
				prominent: false,
			}),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({
				title: `"pipe" left pipes open`,
				severity: "warning",
				prominent: false,
			}),
		);
	});

	it("does not return connectivity issues", () => {
		const issues = getAgentScriptIssues(buildAgent({ status: "disconnected" }));
		expect(issues).not.toContainEqual(
			expect.objectContaining({
				title: "Workspace agent has disconnected",
			}),
		);
	});

	it("returns empty list when no scripts failed", () => {
		expect(
			getAgentScriptIssues(
				buildAgent({ status: "connected", lifecycle_state: "ready" }),
			),
		).toEqual([]);
	});
});

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

	it("returns script issues", () => {
		const issues = getAgentHealthIssues(
			buildAgent(MockWorkspaceAgentStartError),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({
				title: `"Startup Script" failed`,
				severity: "warning",
				prominent: false,
			}),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({
				title: `"time" is taking longer than expected`,
				severity: "warning",
				prominent: false,
			}),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({
				title: `"pipe" left pipes open`,
				severity: "warning",
				prominent: false,
			}),
		);
		expect(
			getAgentHealthIssues(buildAgent(MockWorkspaceAgentStartTimeout)),
		).toContainEqual(
			expect.objectContaining({
				title: `"Startup Script" is taking longer than expected`,
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
			buildAgent({
				...MockWorkspaceAgentStartError,
				status: "disconnected",
			}),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({ title: "Workspace agent has disconnected" }),
		);
		expect(issues).toContainEqual(
			expect.objectContaining({ title: `"Startup Script" failed` }),
		);
	});
});
