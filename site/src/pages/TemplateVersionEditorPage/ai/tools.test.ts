import type { FileTree } from "utils/filetree";
import type { BuildOutput, BuildResult } from "./tools";

vi.mock("ai", () => ({
	tool: (definition: unknown) => definition,
}));

// Import AFTER the mock is set up.
const { createTemplateAgentTools } = await import("./tools");

type ToolSet = ReturnType<typeof createTemplateAgentTools>;

const makeTools = (
	overrides: Record<string, unknown> = {},
	hasBuiltInCurrentRunRef: { current: boolean } = { current: false },
): ToolSet => {
	const fileTree: FileTree = { "main.tf": 'resource "null" {}' };
	return createTemplateAgentTools(
		() => fileTree,
		() => {},
		hasBuiltInCurrentRunRef,
		overrides,
	);
};

// The AI SDK tool context argument. Our mocked tool() is identity,
// so execute is always present but typed as optional. We use a
// minimal stub that satisfies the runtime contract.
const toolContext = {} as never;

// Helper to call tool execute with non-null assertion.
// In tests, tool() is mocked as identity, so execute is always defined.
const executeBuild = (tools: ToolSet) =>
	tools.buildTemplate.execute!({}, toolContext);

const executeGetBuildLogs = (tools: ToolSet) =>
	tools.getBuildLogs.execute!({}, toolContext);

const createDeferred = <T>() => {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((resolvePromise) => {
		resolve = resolvePromise;
	});
	return { promise, resolve };
};

describe("buildTemplate tool", () => {
	it("returns error when onBuildRequested callback is not provided", async () => {
		const tools = makeTools({ waitForBuildComplete: vi.fn() });
		const result = await executeBuild(tools);
		expect(result).toEqual({ error: "Build tools are not available." });
	});

	it("returns error when waitForBuildComplete callback is not provided", async () => {
		const tools = makeTools({ onBuildRequested: vi.fn() });
		const result = await executeBuild(tools);
		expect(result).toEqual({ error: "Build tools are not available." });
	});

	it("returns failed status when onBuildRequested throws", async () => {
		const tools = makeTools({
			onBuildRequested: vi.fn().mockRejectedValue(new Error("Upload failed")),
			waitForBuildComplete: vi.fn(),
		});
		const result = await executeBuild(tools);
		expect(result).toMatchObject({
			status: "failed",
			error: "Upload failed",
		});
	});

	it("calls onBuildRequested then waits for build completion", async () => {
		const buildResult: BuildResult = {
			status: "succeeded",
			logs: "[info] Plan: done",
		};
		const onBuildRequested = vi.fn().mockResolvedValue(undefined);
		const waitForBuildComplete = vi.fn().mockResolvedValue(buildResult);
		const tools = makeTools({ onBuildRequested, waitForBuildComplete });

		const result = await executeBuild(tools);

		expect(onBuildRequested).toHaveBeenCalledTimes(1);
		expect(waitForBuildComplete).toHaveBeenCalledTimes(1);
		expect(result).toEqual(buildResult);
	});

	it.each<BuildResult>([
		{ status: "succeeded", logs: "[info] Plan: done" },
		{
			status: "failed",
			error: "missing provider",
			logs: "[error] missing provider",
		},
	])(
		"clears the timeout when the build resolves with $status",
		async (buildResult) => {
			vi.useFakeTimers();
			try {
				const deferred = createDeferred<BuildResult>();
				const tools = makeTools({
					onBuildRequested: vi.fn().mockResolvedValue(undefined),
					waitForBuildComplete: vi.fn().mockReturnValue(deferred.promise),
				});

				const resultPromise = executeBuild(tools);
				await Promise.resolve();

				expect(vi.getTimerCount()).toBe(1);

				deferred.resolve(buildResult);
				const result = await resultPromise;

				expect(result).toEqual(buildResult);
				expect(vi.getTimerCount()).toBe(0);
			} finally {
				vi.useRealTimers();
			}
		},
	);

	it("returns timeout when build exceeds time limit", async () => {
		vi.useFakeTimers();
		try {
			const neverResolves = new Promise<BuildResult>(() => {});
			const tools = makeTools({
				onBuildRequested: vi.fn().mockResolvedValue(undefined),
				waitForBuildComplete: vi.fn().mockReturnValue(neverResolves),
			});

			const resultPromise = executeBuild(tools);
			await Promise.resolve();

			expect(vi.getTimerCount()).toBe(1);

			await vi.advanceTimersByTimeAsync(180_000);
			const result = await resultPromise;

			expect(result).toMatchObject({ status: "timeout" });
			expect(vi.getTimerCount()).toBe(0);
		} finally {
			vi.useRealTimers();
		}
	});
});

describe("getBuildLogs tool", () => {
	it("returns error when getBuildOutput callback is not provided", async () => {
		const tools = makeTools();
		const result = await executeGetBuildLogs(tools);
		expect(result).toEqual({ error: "Build tools are not available." });
	});

	it("returns no-build status when getBuildOutput returns undefined", async () => {
		const tools = makeTools({
			getBuildOutput: vi.fn().mockReturnValue(undefined),
		});
		const result = await executeGetBuildLogs(tools);
		expect(result).toMatchObject({ status: "none" });
	});

	it("returns current build output when available", async () => {
		const output: BuildOutput = {
			status: "failed",
			error: "missing provider",
			logs: "[error] Plan: missing provider",
		};
		const tools = makeTools({
			getBuildOutput: vi.fn().mockReturnValue(output),
		});
		const result = await executeGetBuildLogs(tools);
		expect(result).toEqual(output);
	});
});

describe("publishTemplate tool", () => {
	const executePublish = (tools: ToolSet, args: Record<string, unknown> = {}) =>
		tools.publishTemplate.execute!(args as never, toolContext);

	it("returns error when onPublishRequested callback is not provided", async () => {
		const tools = makeTools();
		const result = await executePublish(tools);
		expect(result).toEqual({
			success: false,
			error: "Publish is not available.",
		});
	});

	it("returns success when callback resolves with success", async () => {
		const publishResult = { success: true, versionName: "v1.0" };
		const tools = makeTools({
			onPublishRequested: vi.fn().mockResolvedValue(publishResult),
		});
		const result = await executePublish(tools, {
			name: "v1.0",
			message: "Initial release",
			isActiveVersion: true,
		});
		expect(result).toEqual(publishResult);
	});

	it("returns failure when callback throws", async () => {
		const tools = makeTools({
			onPublishRequested: vi
				.fn()
				.mockRejectedValue(new Error("Publish API error")),
		});
		const result = await executePublish(tools);
		expect(result).toEqual({
			success: false,
			error: "Publish API error",
		});
	});

	it("passes publish args through without skipDirtyCheck before a build", async () => {
		const onPublishRequested = vi.fn().mockResolvedValue({ success: true });
		const tools = makeTools({ onPublishRequested });
		await executePublish(tools, {
			name: "my-version",
			message: "changelog entry",
			isActiveVersion: false,
		});
		expect(onPublishRequested).toHaveBeenCalledWith(
			{
				name: "my-version",
				message: "changelog entry",
				isActiveVersion: false,
			},
			undefined,
		);
	});

	it("passes skipDirtyCheck when this run completed a successful build", async () => {
		const onPublishRequested = vi.fn().mockResolvedValue({ success: true });
		const tools = makeTools({
			onBuildRequested: vi.fn().mockResolvedValue(undefined),
			waitForBuildComplete: vi
				.fn()
				.mockResolvedValue({ status: "succeeded", logs: "" }),
			onPublishRequested,
		});

		await executeBuild(tools);
		await executePublish(tools, {
			name: "built-version",
			message: "after build",
			isActiveVersion: true,
		});

		expect(onPublishRequested).toHaveBeenCalledWith(
			{
				name: "built-version",
				message: "after build",
				isActiveVersion: true,
			},
			{ skipDirtyCheck: true },
		);
	});

	it("reuses a shared build-state ref across tool instances", async () => {
		const hasBuiltInCurrentRunRef = { current: false };
		const onBuildRequested = vi.fn().mockResolvedValue(undefined);
		const waitForBuildComplete = vi
			.fn()
			.mockResolvedValue({ status: "succeeded", logs: "" });
		const onPublishRequested = vi.fn().mockResolvedValue({ success: true });

		const buildTools = makeTools(
			{
				onBuildRequested,
				waitForBuildComplete,
				onPublishRequested,
			},
			hasBuiltInCurrentRunRef,
		);
		await executeBuild(buildTools);

		const publishTools = makeTools(
			{
				onBuildRequested,
				waitForBuildComplete,
				onPublishRequested,
			},
			hasBuiltInCurrentRunRef,
		);
		await executePublish(publishTools, {
			name: "built-version",
			message: "after approval",
			isActiveVersion: true,
		});

		expect(onPublishRequested).toHaveBeenLastCalledWith(
			{
				name: "built-version",
				message: "after approval",
				isActiveVersion: true,
			},
			{ skipDirtyCheck: true },
		);
	});
});
