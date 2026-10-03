import { describe, expect, it } from "vitest";
import type { Workspace, WorkspaceApp } from "#/api/typesGenerated";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import {
	isChatAgentBindingUnresolved,
	isWatchedWorkspaceViewUnchanged,
} from "./watchedWorkspace";

describe("isWatchedWorkspaceViewUnchanged", () => {
	const cloneWithApps = (apps: WorkspaceApp[]): Workspace => ({
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: MockWorkspace.latest_build.resources.map((resource) => ({
				...resource,
				agents: resource.agents?.map((agent) =>
					agent.id === MockWorkspaceAgent.id ? { ...agent, apps } : agent,
				),
			})),
		},
	});

	it("is true for a fresh payload with only unwatched changes", () => {
		const next: Workspace = {
			...MockWorkspace,
			last_used_at: "2024-01-01T00:00:00Z",
		};

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(true);
	});

	it("is false when a bound-agent app changes health", () => {
		const next = cloneWithApps([{ ...MockWorkspaceApp, health: "healthy" }]);

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(false);
	});

	it("is false when the bound agent gains an app", () => {
		const next = cloneWithApps([
			MockWorkspaceApp,
			{ ...MockWorkspaceApp, id: "second-app", slug: "second-app" },
		]);

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(false);
	});

	it("is false when the latest build changes", () => {
		const next: Workspace = {
			...MockWorkspace,
			latest_build: { ...MockWorkspace.latest_build, id: "new-build-id" },
		};

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(false);
	});
});

describe("isChatAgentBindingUnresolved", () => {
	it("is true when the bound agent is missing from the running build", () => {
		expect(isChatAgentBindingUnresolved(MockWorkspace, "stale-agent-id")).toBe(
			true,
		);
	});

	it("is true when the chat has no binding yet", () => {
		expect(isChatAgentBindingUnresolved(MockWorkspace, undefined)).toBe(true);
	});

	it("is false when the bound agent resolves", () => {
		expect(
			isChatAgentBindingUnresolved(MockWorkspace, MockWorkspaceAgent.id),
		).toBe(false);
	});

	it("is false when the workspace is not running", () => {
		const stopped: Workspace = {
			...MockWorkspace,
			latest_build: { ...MockWorkspace.latest_build, status: "stopped" },
		};

		expect(isChatAgentBindingUnresolved(stopped, "stale-agent-id")).toBe(false);
	});

	it("is false when the running build has no agents", () => {
		const noAgents: Workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				resources: MockWorkspace.latest_build.resources.map((resource) => ({
					...resource,
					agents: [],
				})),
			},
		};

		expect(isChatAgentBindingUnresolved(noAgents, "stale-agent-id")).toBe(
			false,
		);
	});

	it("is false while the workspace is loading", () => {
		expect(isChatAgentBindingUnresolved(undefined, "stale-agent-id")).toBe(
			false,
		);
	});
});
