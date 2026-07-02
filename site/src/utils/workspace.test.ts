import dayjs from "dayjs";
import type * as TypesGen from "#/api/typesGenerated";
import * as Mocks from "#/testHelpers/entities";
import {
	agentVersionStatus,
	defaultWorkspaceExtension,
	findWorkspaceAgent,
	getDisplayVersionStatus,
	getDisplayWorkspaceBuildInitiatedBy,
	getDisplayWorkspaceTemplateName,
	getFirstRootAgent,
	getMatchingAgentOrFirst,
	getWorkspaceAgents,
	isWorkspaceOn,
} from "./workspace";

function buildWorkspace(
	resourceAgents: readonly TypesGen.WorkspaceAgent[][],
): TypesGen.Workspace {
	const resourceTemplate = Mocks.MockWorkspace.latest_build.resources[0];
	return {
		...Mocks.MockWorkspace,
		latest_build: {
			...Mocks.MockWorkspace.latest_build,
			resources: resourceAgents.map((agents) => ({
				...resourceTemplate,
				agents,
			})),
		},
	};
}

function buildAgent(
	id: string,
	parentId: string | null = null,
): TypesGen.WorkspaceAgent {
	return {
		...Mocks.MockWorkspaceAgent,
		id,
		name: id,
		parent_id: parentId,
	};
}

describe("util > workspace", () => {
	describe("isWorkspaceOn", () => {
		it.each<
			[TypesGen.WorkspaceTransition, TypesGen.ProvisionerJobStatus, boolean]
		>([
			["delete", "canceled", false],
			["delete", "canceling", false],
			["delete", "failed", false],
			["delete", "pending", false],
			["delete", "running", false],
			["delete", "succeeded", false],

			["stop", "canceled", false],
			["stop", "canceling", false],
			["stop", "failed", false],
			["stop", "pending", false],
			["stop", "running", false],
			["stop", "succeeded", false],

			["start", "canceled", false],
			["start", "canceling", false],
			["start", "failed", false],
			["start", "pending", false],
			["start", "running", false],
			["start", "succeeded", true],
		])("transition=%p, status=%p, isWorkspaceOn=%p", (transition, status, isOn) => {
			const workspace: TypesGen.Workspace = {
				...Mocks.MockWorkspace,
				latest_build: {
					...Mocks.MockWorkspaceBuild,
					job: {
						...Mocks.MockProvisionerJob,
						status,
					},
					transition,
				},
			};
			expect(isWorkspaceOn(workspace)).toBe(isOn);
		});
	});

	describe("defaultWorkspaceExtension", () => {
		it.each<[string, TypesGen.PutExtendWorkspaceRequest]>([
			[
				"2022-06-02T14:56:34Z",
				{
					deadline: "2022-06-02T18:56:34Z",
				},
			],

			// This case is the same as above, but in a different timezone to prove
			// that UTC conversion for deadline works as expected
			[
				"2022-06-02T10:56:20-04:00",
				{
					deadline: "2022-06-02T18:56:20Z",
				},
			],
		])("defaultWorkspaceExtension(%p) returns %p", (startTime, request) => {
			expect(defaultWorkspaceExtension(dayjs(startTime))).toEqual(request);
		});
	});

	describe("getDisplayWorkspaceBuildInitiatedBy", () => {
		it.each<[TypesGen.WorkspaceBuild, string]>([
			[Mocks.MockWorkspaceBuild, "TestUser"],
			[
				{
					...Mocks.MockWorkspaceBuild,
					reason: "autostart",
				},
				"Coder",
			],
			[
				{
					...Mocks.MockWorkspaceBuild,
					reason: "autostop",
				},
				"Coder",
			],
			[
				{
					...Mocks.MockWorkspaceBuild,
					reason: "task_auto_pause",
				},
				"Coder",
			],
		])("getDisplayWorkspaceBuildInitiatedBy(%p) returns %p", (build, initiatedBy) => {
			expect(getDisplayWorkspaceBuildInitiatedBy(build)).toEqual(initiatedBy);
		});
	});

	describe("getDisplayVersionStatus", () => {
		it.each<[string, string, string, string, string, agentVersionStatus]>([
			["", "", "", "", "Unknown", agentVersionStatus.Updated],
			["", "v1.2.3", "", "", "Unknown", agentVersionStatus.Updated],
			["v1.2.3", "", "", "", "v1.2.3", agentVersionStatus.Updated],
			["v1.2.3", "v1.2.3", "", "", "v1.2.3", agentVersionStatus.Updated],
			["v1.2.3", "v1.2.4", "", "", "v1.2.3", agentVersionStatus.Outdated],
			["v1.2.4", "v1.2.3", "", "", "v1.2.4", agentVersionStatus.Updated],
			["foo", "bar", "", "", "foo", agentVersionStatus.Updated],
			[
				"v1.2.3",
				"v1.2.4",
				"1.8",
				"2.1",
				"v1.2.3",
				agentVersionStatus.Deprecated,
			],
		])("getDisplayVersionStatus(theme, %p, %p, %p, %p) returns (%p, %p)", (agentVersion, serverVersion, agentAPIVersion, serverAPIVersion, expectedVersion, expectedStatus) => {
			const { displayVersion, status } = getDisplayVersionStatus(
				agentVersion,
				serverVersion,
				agentAPIVersion,
				serverAPIVersion,
			);
			expect(displayVersion).toEqual(expectedVersion);
			expect(status).toEqual(expectedStatus);
		});
	});

	describe("getDisplayWorkspaceTemplateName", () => {
		it("display name is not set", async () => {
			const workspace: TypesGen.Workspace = {
				...Mocks.MockWorkspace,
				template_display_name: "",
			};
			const displayed = getDisplayWorkspaceTemplateName(workspace);
			expect(displayed).toEqual(workspace.template_name);
		});
		it("display name is set", async () => {
			const workspace: TypesGen.Workspace = {
				...Mocks.MockWorkspace,
			};
			const displayed = getDisplayWorkspaceTemplateName(workspace);
			expect(displayed).toEqual(workspace.template_display_name);
		});
	});

	describe("getWorkspaceAgents", () => {
		it("flattens agents across workspace resources", () => {
			const workspace = buildWorkspace([
				[buildAgent("agent-1")],
				[buildAgent("agent-2")],
			]);

			expect(getWorkspaceAgents(workspace).map((agent) => agent.id)).toEqual([
				"agent-1",
				"agent-2",
			]);
		});
	});

	describe("findWorkspaceAgent", () => {
		it("returns the matching agent by ID", () => {
			const workspace = buildWorkspace([[buildAgent("agent-1")]]);

			expect(findWorkspaceAgent(workspace, "agent-1")?.name).toBe("agent-1");
			expect(findWorkspaceAgent(workspace, "missing")).toBeUndefined();
		});
	});

	describe("getFirstRootAgent", () => {
		it("skips sub-agents and returns the first root agent", () => {
			const workspace = buildWorkspace([
				[buildAgent("sub-agent", "root-agent")],
				[buildAgent("root-agent")],
			]);

			expect(getFirstRootAgent(workspace)?.id).toBe("root-agent");
		});

		it("falls back to the first agent when all agents are sub-agents", () => {
			const workspace = buildWorkspace([
				[buildAgent("sub-agent-1", "missing")],
				[buildAgent("sub-agent-2", "missing")],
			]);

			expect(getFirstRootAgent(workspace)?.id).toBe("sub-agent-1");
		});

		it("returns undefined when there are no agents", () => {
			const workspace = buildWorkspace([[]]);

			expect(getFirstRootAgent(workspace)).toBeUndefined();
		});
	});

	describe("getMatchingAgentOrFirst", () => {
		it("returns the agent matching by name across resources", () => {
			const workspace = buildWorkspace([
				[buildAgent("agent-1")],
				[buildAgent("agent-2")],
			]);

			expect(getMatchingAgentOrFirst(workspace, "agent-2")?.id).toBe("agent-2");
		});

		it("returns the first agent when no name is given", () => {
			const workspace = buildWorkspace([
				[buildAgent("agent-1")],
				[buildAgent("agent-2")],
			]);

			expect(getMatchingAgentOrFirst(workspace, undefined)?.id).toBe("agent-1");
		});

		it("prefers a root agent over a sub-agent when no name is given", () => {
			const workspace = buildWorkspace([
				[buildAgent("sub-agent", "root-agent")],
				[buildAgent("root-agent")],
			]);

			expect(getMatchingAgentOrFirst(workspace, undefined)?.id).toBe(
				"root-agent",
			);
		});

		it("still matches a sub-agent by name", () => {
			const workspace = buildWorkspace([
				[buildAgent("sub-agent", "root-agent")],
				[buildAgent("root-agent")],
			]);

			expect(getMatchingAgentOrFirst(workspace, "sub-agent")?.id).toBe(
				"sub-agent",
			);
		});

		it("returns undefined when no agent matches the name", () => {
			const workspace = buildWorkspace([[buildAgent("agent-1")]]);

			expect(getMatchingAgentOrFirst(workspace, "missing")).toBeUndefined();
		});
	});
});
