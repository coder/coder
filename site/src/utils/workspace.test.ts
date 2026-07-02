import dayjs from "dayjs";
import type * as TypesGen from "#/api/typesGenerated";
import * as Mocks from "#/testHelpers/entities";
import {
	agentVersionStatus,
	defaultWorkspaceExtension,
	findChatAgent,
	findWorkspaceAgent,
	getDisplayVersionStatus,
	getDisplayWorkspaceBuildInitiatedBy,
	getDisplayWorkspaceTemplateName,
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

	// Scenarios mirror coderd/x/chatd/agentselect/agentselect_test.go where
	// practical.
	describe("findChatAgent", () => {
		const agentWithId = (
			id: string,
			name: string,
			displayOrder = 0,
			parentId: string | null = null,
		): TypesGen.WorkspaceAgent => ({
			...Mocks.MockWorkspaceAgent,
			id,
			name,
			display_order: displayOrder,
			parent_id: parentId,
		});

		const agent = (
			name: string,
			displayOrder = 0,
			parentId: string | null = null,
		): TypesGen.WorkspaceAgent =>
			agentWithId(crypto.randomUUID(), name, displayOrder, parentId);

		it("returns the single agent matching the chat suffix", () => {
			const workspace = buildWorkspace([
				[agent("alpha"), agent("dev-coderd-chat"), agent("zeta")],
			]);

			expect(findChatAgent(workspace)?.name).toBe("dev-coderd-chat");
		});

		it("matches the chat suffix case-insensitively", () => {
			const workspace = buildWorkspace([
				[agent("alpha"), agent("Dev-Coderd-Chat"), agent("zeta")],
			]);

			expect(findChatAgent(workspace)?.name).toBe("Dev-Coderd-Chat");
		});

		it("falls back to the lowest display order when no suffix matches", () => {
			const workspace = buildWorkspace([
				[agent("zeta", 2), agent("bravo", 1), agent("alpha", 1)],
			]);

			expect(findChatAgent(workspace)?.name).toBe("alpha");
		});

		it("falls back to the first root agent by case-insensitive name", () => {
			const workspace = buildWorkspace([
				[agent("Bravo", 3), agent("alpha", 3), agent("charlie", 3)],
			]);

			expect(findChatAgent(workspace)?.name).toBe("alpha");
		});

		it("prefers a lower display order over name ordering", () => {
			const workspace = buildWorkspace([[agent("zeta", 0), agent("alpha", 1)]]);

			expect(findChatAgent(workspace)?.name).toBe("zeta");
		});

		it("breaks case-only name ties by case-sensitive name", () => {
			const workspace = buildWorkspace([[agent("dev"), agent("Dev")]]);

			expect(findChatAgent(workspace)?.name).toBe("Dev");
		});

		it("breaks exact name ties by ID", () => {
			const workspace = buildWorkspace([
				[
					agentWithId("00000000-0000-0000-0000-000000000002", "dev"),
					agentWithId("00000000-0000-0000-0000-000000000001", "dev"),
				],
			]);

			expect(findChatAgent(workspace)?.id).toBe(
				"00000000-0000-0000-0000-000000000001",
			);
		});

		it("returns undefined when multiple root agents match the suffix", () => {
			const workspace = buildWorkspace([
				[agent("alpha-coderd-chat"), agent("beta-coderd-chat"), agent("gamma")],
			]);

			expect(findChatAgent(workspace)).toBeUndefined();
		});

		it("ignores sub-agents matching the suffix", () => {
			const workspace = buildWorkspace([
				[
					agent("alpha", 1),
					agent("child-coderd-chat", 0, "some-parent"),
					agent("bravo", 0),
				],
			]);

			expect(findChatAgent(workspace)?.name).toBe("bravo");
		});

		it("prefers a root suffix match over a sub-agent suffix match", () => {
			const workspace = buildWorkspace([
				[
					agent("alpha"),
					agent("child-coderd-chat", 1, "some-parent"),
					agent("root-coderd-chat", 2),
				],
			]);

			expect(findChatAgent(workspace)?.name).toBe("root-coderd-chat");
		});

		it("returns undefined when there are no agents", () => {
			const workspace = buildWorkspace([[]]);

			expect(findChatAgent(workspace)).toBeUndefined();
		});

		it("returns undefined when only sub-agents exist", () => {
			const workspace = buildWorkspace([
				[
					agent("alpha", 0, "some-parent"),
					agent("beta-coderd-chat", 1, "some-parent"),
				],
			]);

			expect(findChatAgent(workspace)).toBeUndefined();
		});

		it("returns a single root agent", () => {
			const workspace = buildWorkspace([[agent("solo", 5)]]);

			expect(findChatAgent(workspace)?.name).toBe("solo");
		});

		it("picks the suffix agent regardless of sort position", () => {
			const workspace = buildWorkspace([
				[
					agent("alpha", 0),
					agent("zeta", 1),
					agent("preferred-coderd-chat", 99),
				],
			]);

			expect(findChatAgent(workspace)?.name).toBe("preferred-coderd-chat");
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
