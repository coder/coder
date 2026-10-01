import { describe, expect, it } from "vitest";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceBuild,
} from "#/api/typesGenerated";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import {
	getWorkspaceToolStage,
	type WorkspaceToolAction,
} from "./workspaceToolStage";

const buildId = MockWorkspace.latest_build.id;

const workspaceWith = (
	build: Partial<WorkspaceBuild>,
	agent?: Partial<WorkspaceAgent>,
): Workspace => ({
	...MockWorkspace,
	latest_build: {
		...MockWorkspace.latest_build,
		...build,
		resources: [
			{
				...MockWorkspace.latest_build.resources[0],
				agents: agent ? [{ ...MockWorkspaceAgent, ...agent }] : [],
			},
		],
	},
});

describe("getWorkspaceToolStage", () => {
	it.each<{
		case: string;
		action: WorkspaceToolAction;
		workspace: Workspace | undefined;
		callBuildId?: string;
		stage: string | undefined;
	}>([
		{
			case: "no workspace",
			action: "start",
			workspace: undefined,
			callBuildId: buildId,
			stage: undefined,
		},
		{
			case: "no call build",
			action: "start",
			workspace: workspaceWith({ status: "starting" }),
			stage: undefined,
		},
		{
			case: "latest build is another build",
			action: "start",
			workspace: workspaceWith({ id: "other", status: "starting" }),
			callBuildId: buildId,
			stage: undefined,
		},
		{
			case: "start row following a stop build",
			action: "start",
			workspace: workspaceWith({ transition: "stop", status: "stopping" }),
			callBuildId: buildId,
			stage: undefined,
		},
		{
			case: "stop row following a start build",
			action: "stop",
			workspace: workspaceWith({ transition: "start", status: "starting" }),
			callBuildId: buildId,
			stage: undefined,
		},
		{
			case: "start build queued",
			action: "start",
			workspace: workspaceWith({ transition: "start", status: "pending" }),
			callBuildId: buildId,
			stage: "Waiting in build queue…",
		},
		{
			case: "stop build queued",
			action: "stop",
			workspace: workspaceWith({ transition: "stop", status: "pending" }),
			callBuildId: buildId,
			stage: "Waiting in build queue…",
		},
		{
			case: "stop build running",
			action: "stop",
			workspace: workspaceWith({ transition: "stop", status: "stopping" }),
			callBuildId: buildId,
			stage: "Stopping workspace…",
		},
		{
			case: "stop build finished",
			action: "stop",
			workspace: workspaceWith({ transition: "stop", status: "stopped" }),
			callBuildId: buildId,
			stage: undefined,
		},
		{
			case: "create build running",
			action: "create",
			workspace: workspaceWith({ transition: "start", status: "starting" }),
			callBuildId: buildId,
			stage: "Building workspace…",
		},
		{
			case: "chat agent not in the build yet",
			action: "start",
			workspace: workspaceWith({ transition: "start", status: "running" }),
			callBuildId: buildId,
			stage: "Waiting for workspace agent to connect…",
		},
		{
			case: "agent connecting",
			action: "start",
			workspace: workspaceWith(
				{ transition: "start", status: "running" },
				{ status: "connecting", lifecycle_state: "created" },
			),
			callBuildId: buildId,
			stage: "Waiting for workspace agent to connect…",
		},
		{
			case: "agent connected, scripts running",
			action: "create",
			workspace: workspaceWith(
				{ transition: "start", status: "running" },
				{ status: "connected", lifecycle_state: "starting" },
			),
			callBuildId: buildId,
			stage: "Running startup scripts…",
		},
		{
			case: "agent connected, lifecycle created",
			action: "start",
			workspace: workspaceWith(
				{ transition: "start", status: "running" },
				{ status: "connected", lifecycle_state: "created" },
			),
			callBuildId: buildId,
			stage: "Running startup scripts…",
		},
		{
			case: "agent ready",
			action: "start",
			workspace: workspaceWith(
				{ transition: "start", status: "running" },
				{ status: "connected", lifecycle_state: "ready" },
			),
			callBuildId: buildId,
			stage: undefined,
		},
		{
			case: "build failed",
			action: "start",
			workspace: workspaceWith({ transition: "start", status: "failed" }),
			callBuildId: buildId,
			stage: undefined,
		},
	])("$case", ({ action, workspace, callBuildId, stage }) => {
		expect(
			getWorkspaceToolStage({
				action,
				workspace,
				callBuildId,
				chatAgentId: MockWorkspaceAgent.id,
			}),
		).toBe(stage);
	});
});
