import { describe, expect, it } from "vitest";
import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";
import { MockFailedWorkspace } from "#/testHelpers/entities";
import {
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildLogsMaxBytes,
	debugWorkspaceBuildPrompt,
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
		error_code: "REQUIRED_TEMPLATE_VARIABLES",
		logs_overflowed: false,
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

const logLines = (count: number, output: (index: number) => string) =>
	Array.from(
		{ length: count },
		(_, index): ProvisionerJobLog => ({
			id: index,
			created_at: "2024-01-01T00:00:00.000Z",
			log_source: "provisioner",
			log_level: "info",
			stage: "Starting workspace",
			output: output(index),
		}),
	);

const byteLength = (text: string) => new TextEncoder().encode(text).length;

describe("debugWorkspaceBuildPrompt", () => {
	it("names the workspace and the failed transition", () => {
		const prompt = debugWorkspaceBuildPrompt({
			...failedBuild,
			transition: "stop",
		});

		expect(
			prompt.startsWith("Workspace dfraley/my-workspace failed to stop."),
		).toBe(true);
		expect(prompt.match(/failed to stop/g)).toHaveLength(2);
		expect(prompt).not.toContain("failed to start");
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
				"Job error code: REQUIRED_TEMPLATE_VARIABLES",
				"Job error: terraform plan: exit status 1",
				"",
				"Build logs:",
				"",
				"=== Setting up (2024-01-01T00:00:00.000Z) ===",
				"[info] ",
				"",
				"=== Planning infrastructure (2024-01-01T00:00:01.000Z) ===",
				"[debug] Initializing the backend...",
				"[error] Error: Invalid value for variable",
				"",
			].join("\n"),
		);
	});

	it("notes when no logs were recorded", () => {
		const text = formatWorkspaceBuildLogsForDebug(failedBuild, []);

		expect(text).toContain("(no build logs were recorded)");
	});

	it("notes when the provisioner log limit truncated the build", () => {
		const text = formatWorkspaceBuildLogsForDebug(
			{ ...failedBuild, job: { ...failedBuild.job, logs_overflowed: true } },
			logs,
		);

		expect(text).toContain("Note: the build hit the provisioner log limit");
	});

	it("keeps the header and the most recent lines within the byte budget", () => {
		// Terraform diagnostics prefix lines with a 3-byte character.
		const text = formatWorkspaceBuildLogsForDebug(
			failedBuild,
			logLines(3000, (index) => `│ ${index} ${"x".repeat(96)}`),
		);

		expect(byteLength(text)).toBeLessThanOrEqual(
			debugWorkspaceBuildLogsMaxBytes,
		);
		expect(text).toContain("Job error: terraform plan: exit status 1");
		expect(text).toContain(
			"=== Starting workspace (2024-01-01T00:00:00.000Z) ===",
		);
		expect(text).toMatch(
			/\[\d+ earlier lines omitted to fit the attachment size limit\]/,
		);
		expect(text).not.toContain("[info] │ 0 x");
		expect(text).toContain(`[info] │ 2999 ${"x".repeat(96)}`);
	});

	it("counts only log lines as omitted and relabels the surviving stage", () => {
		// Two of these fit within the budget; the third does not.
		const bigLine = "z".repeat(
			Math.ceil(debugWorkspaceBuildLogsMaxBytes * 0.55),
		);
		const text = formatWorkspaceBuildLogsForDebug(failedBuild, [
			...logLines(3, () => bigLine),
			...logLines(2, (index) => `kept ${index}`).map((log) => ({
				...log,
				stage: "Cleaning up",
			})),
		]);

		expect(text).toContain(
			"[2 earlier lines omitted to fit the attachment size limit]",
		);
		expect(
			text.split("=== Cleaning up (2024-01-01T00:00:00.000Z) ==="),
		).toHaveLength(2);
		expect(text).toMatch(
			/\[2 earlier lines omitted to fit the attachment size limit\]\n=== Starting workspace \(2024-01-01T00:00:00.000Z\) ===\n\[info\] zzz/,
		);
	});

	it("keeps the tail of a single line that is over budget", () => {
		const text = formatWorkspaceBuildLogsForDebug(failedBuild, [
			...logLines(1, () => "first"),
			...logLines(
				1,
				() => `${"a".repeat(debugWorkspaceBuildLogsMaxBytes * 2)}END`,
			),
		]);

		expect(byteLength(text)).toBeLessThanOrEqual(
			debugWorkspaceBuildLogsMaxBytes,
		);
		expect(text).toContain("Job error: terraform plan: exit status 1");
		expect(text).toContain(
			"[1 earlier lines omitted to fit the attachment size limit]",
		);
		expect(text).toContain(
			"[the start of the next line was omitted to fit the attachment size limit]",
		);
		expect(text.endsWith("aaaEND\n")).toBe(true);
		expect(text).not.toContain("[info] first");
	});

	// The cut lands inside a 3-byte character in two of every three offsets.
	it.each(["END", "END1", "END12"])(
		"drops a partial character at the cut (%s)",
		(suffix) => {
			const text = formatWorkspaceBuildLogsForDebug(
				failedBuild,
				logLines(
					1,
					() => `${"│".repeat(debugWorkspaceBuildLogsMaxBytes)}${suffix}`,
				),
			);

			expect(text).not.toContain("\uFFFD");
			expect(text.endsWith(`│${suffix}\n`)).toBe(true);
		},
	);

	it("caps an oversized job error in the header", () => {
		const text = formatWorkspaceBuildLogsForDebug(
			{
				...failedBuild,
				job: {
					...failedBuild.job,
					error: `summary first ${"e".repeat(debugWorkspaceBuildLogsMaxBytes * 2)}`,
				},
			},
			logs,
		);

		expect(byteLength(text)).toBeLessThanOrEqual(
			debugWorkspaceBuildLogsMaxBytes,
		);
		expect(text).toContain("Job error: summary first eee");
		expect(text).toContain("(error truncated)");
		expect(text).toContain("[error] Error: Invalid value for variable");
	});
});
