import { describe, expect, it } from "vitest";
import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";
import { MockFailedWorkspace } from "#/testHelpers/entities";
import {
	buildDebugWorkspaceBuildPath,
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildSearchParam,
	formatWorkspaceBuildLogsForDebug,
} from "./workspaceBuildDebug";

const failedBuild: WorkspaceBuild = {
	...MockFailedWorkspace.latest_build,
	workspace_owner_name: "dfraley",
	workspace_name: "my-workspace",
	template_version_name: "v1.2.3",
	build_number: 7,
	transition: "start",
	reason: "initiator",
	status: "failed",
	job: {
		...MockFailedWorkspace.latest_build.job,
		status: "failed",
		error: "terraform plan: exit status 1",
		error_code: undefined,
	},
};

const logs: ProvisionerJobLog[] = [
	{
		id: 1,
		created_at: "2024-01-01T00:00:00.000Z",
		log_source: "provisioner_daemon",
		log_level: "info",
		stage: "Setting up",
		output: "",
	},
	{
		id: 2,
		created_at: "2024-01-01T00:00:01.000Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Planning infrastructure",
		output: "Initializing the backend...",
	},
	{
		id: 3,
		created_at: "2024-01-01T00:00:02.000Z",
		log_source: "provisioner",
		log_level: "error",
		stage: "Planning infrastructure",
		output: "Error: Invalid value for variable",
	},
];

describe("buildDebugWorkspaceBuildPath", () => {
	it("links to the agents create page with the build id", () => {
		const path = buildDebugWorkspaceBuildPath("build id/with?chars");
		const url = new URL(path, "https://coder.example.com");

		expect(url.pathname).toBe("/agents");
		expect(url.searchParams.get(debugWorkspaceBuildSearchParam)).toBe(
			"build id/with?chars",
		);
	});
});

describe("debugWorkspaceBuildLogsFileName", () => {
	it("names the attachment after the workspace and build number", () => {
		expect(debugWorkspaceBuildLogsFileName(failedBuild)).toBe(
			"workspace-build-logs-dfraley-my-workspace-7.txt",
		);
	});
});

describe("formatWorkspaceBuildLogsForDebug", () => {
	it("includes the build summary, job error, and logs grouped by stage", () => {
		const text = formatWorkspaceBuildLogsForDebug(failedBuild, logs);

		expect(text).toBe(
			[
				"Workspace: dfraley/my-workspace",
				"Template version: v1.2.3",
				"Build: #7 (start, reason: initiator)",
				"Build status: failed",
				"Job status: failed",
				"Job error: terraform plan: exit status 1",
				"",
				"Build logs:",
				"",
				"=== Setting up ===",
				"2024-01-01T00:00:00.000Z [info] ",
				"",
				"=== Planning infrastructure ===",
				"2024-01-01T00:00:01.000Z [debug] Initializing the backend...",
				"2024-01-01T00:00:02.000Z [error] Error: Invalid value for variable",
				"",
			].join("\n"),
		);
	});

	it("notes when no logs were recorded", () => {
		const text = formatWorkspaceBuildLogsForDebug(failedBuild, []);

		expect(text).toContain("(no build logs were recorded)");
	});
});
