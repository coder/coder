import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { expect, it, vi } from "vitest";
import {
	MockACPSession,
	MockACPSessionArgs,
	MockACPSpawnArgs,
	MockACPTranscriptDeltas,
	MockACPWaitResult,
} from "#/testHelpers/acp";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { applyMessagePartToStreamState } from "../../ChatConversation/streamState";
import {
	ACPContext,
	acpSessionKey,
	buildACPSessionDescriptors,
} from "./ACPContext";
import { ACPTool } from "./ACPTool";
import { ACPToolNames } from "./acpToolNames";

it("updates nested tool activity while the wait is running", () => {
	const ToolComponent = vi.fn(() => null);
	const props = {
		ToolComponent,
		organizationId: MockChatModel.organization_id,
		mcpServers: [],
		subagentTitles: new Map(),
		subagentVariants: new Map(),
		shellToolDisplayMode: "always_collapsed",
		codeDiffDisplayMode: "always_expanded",
		name: ACPToolNames.WaitAgent,
		status: "running",
		args: MockACPSessionArgs,
		isError: false,
	} as const;
	const queryClient = createTestQueryClient();
	const activity = (result: unknown) => (
		<QueryClientProvider client={queryClient}>
			<ACPTool {...props} result={result} />
		</QueryClientProvider>
	);
	const waiting = applyMessagePartToStreamState(null, {
		type: "tool-result",
		tool_call_id: "wait",
		tool_name: ACPToolNames.WaitAgent,
		result_delta: MockACPTranscriptDeltas.slice(0, 3).join(""),
	});
	const { rerender } = renderComponent(
		activity(waiting?.toolResults.wait.result),
	);
	expect(ToolComponent).toHaveBeenLastCalledWith(
		expect.objectContaining({
			name: "Run unit tests",
			status: "running",
			organizationId: props.organizationId,
			mcpServers: props.mcpServers,
			subagentTitles: props.subagentTitles,
			subagentVariants: props.subagentVariants,
			shellToolDisplayMode: props.shellToolDisplayMode,
			codeDiffDisplayMode: props.codeDiffDisplayMode,
		}),
		undefined,
	);
	const completed = applyMessagePartToStreamState(waiting, {
		type: "tool-result",
		tool_call_id: "wait",
		result_delta: MockACPTranscriptDeltas.slice(3).join(""),
	});
	rerender(activity(completed?.toolResults.wait.result));
	expect(ToolComponent).toHaveBeenLastCalledWith(
		expect.objectContaining({
			name: "Run unit tests",
			status: "completed",
			result: "ok github.com/coder/coder/v2/agent",
		}),
		undefined,
	);
});

it("preserves the user's collapsed state when a wait result changes", async () => {
	const ToolComponent = vi.fn(() => null);
	const mockRenamedSession = {
		...MockACPSession,
		harness_display_name: "Workspace Assistant",
	};
	const mockWaitResult = { ...MockACPWaitResult, ...mockRenamedSession };
	const sessions = buildACPSessionDescriptors([
		{
			id: "spawn",
			name: ACPToolNames.SpawnAgent,
			status: "completed",
			isError: false,
			args: MockACPSpawnArgs,
			result: mockRenamedSession,
		},
	]);
	const queryClient = createTestQueryClient();
	const activity = (result: unknown) => (
		<QueryClientProvider client={queryClient}>
			<ACPContext value={sessions}>
				<ACPTool
					ToolComponent={ToolComponent}
					organizationId={MockChatModel.organization_id}
					mcpServers={[]}
					subagentTitles={new Map()}
					subagentVariants={new Map()}
					shellToolDisplayMode="auto"
					codeDiffDisplayMode="auto"
					name={ACPToolNames.WaitAgent}
					status="completed"
					args={MockACPSessionArgs}
					result={result}
					isError={false}
				/>
			</ACPContext>
		</QueryClientProvider>
	);
	const { rerender } = renderComponent(activity(mockWaitResult));
	expect(ToolComponent).toHaveBeenCalledWith(
		expect.objectContaining({
			name: "Run unit tests",
			args: { command: "go test ./agent/..." },
			result: "ok github.com/coder/coder/v2/agent",
			status: "completed",
			isError: false,
		}),
		undefined,
	);
	await userEvent.click(
		screen.getByRole("button", { name: /^Waited for Workspace Assistant/ }),
	);
	ToolComponent.mockClear();
	rerender(activity({ output: "Checking tests" }));
	await userEvent.click(
		screen.getByRole("button", { name: /^Waited for Workspace Assistant/ }),
	);
	await userEvent.click(
		screen.getByRole("button", { name: /^Waited for Workspace Assistant/ }),
	);
	rerender(activity({ ...mockWaitResult, timed_out: true }));
	expect(ToolComponent).not.toHaveBeenCalled();
	await userEvent.click(
		screen.getByRole("button", { name: /^Waited for Workspace Assistant/ }),
	);
	expect(ToolComponent).toHaveBeenCalledWith(
		expect.objectContaining({ name: "Run unit tests" }),
		undefined,
	);
});

it("recovers session labels without replacing the original task on follow-up messages", () => {
	expect(
		buildACPSessionDescriptors([
			{
				id: "spawn",
				name: ACPToolNames.SpawnAgent,
				status: "completed",
				isError: false,
				args: MockACPSpawnArgs,
				result: MockACPSession,
			},
			{
				id: "message",
				name: ACPToolNames.MessageAgent,
				status: "completed",
				isError: false,
				args: { ...MockACPSessionArgs, message: "A new task" },
				result: MockACPSessionArgs,
			},
		]).get(acpSessionKey(MockACPSessionArgs, undefined)),
	).toEqual({
		displayName: MockACPSession.harness_display_name,
		prompt: MockACPSpawnArgs.prompt,
	});
});

it("recovers session labels from follow-up results when spawn is outside the loaded history", () => {
	expect(
		buildACPSessionDescriptors([
			{
				id: "message",
				name: ACPToolNames.MessageAgent,
				status: "completed",
				isError: false,
				args: { ...MockACPSessionArgs, message: "Check tests" },
				result: { ...MockACPSessionArgs, ...MockACPSession },
			},
			{
				id: "wait",
				name: ACPToolNames.WaitAgent,
				status: "running",
				isError: false,
				args: MockACPSessionArgs,
				result: { messages: [] },
			},
		]).get(acpSessionKey(MockACPSessionArgs, undefined)),
	).toEqual({ displayName: MockACPSession.harness_display_name, prompt: "" });
});

it("distinguishes matching native session IDs in different harnesses and directories", () => {
	const variants = [
		{ args: MockACPSpawnArgs, result: MockACPSession },
		{
			args: {
				...MockACPSpawnArgs,
				agent: { harness_id: "codex" },
				prompt: "Codex task",
			},
			result: {
				...MockACPSession,
				harness_id: "codex",
				harness_display_name: "Codex",
			},
		},
		{
			args: {
				...MockACPSpawnArgs,
				working_directory: "/other",
				prompt: "Other directory",
			},
			result: MockACPSession,
		},
	];
	const descriptors = buildACPSessionDescriptors(
		variants.map((variant, index) => ({
			...variant,
			id: `spawn-${index}`,
			name: ACPToolNames.SpawnAgent,
			status: "completed",
			isError: false,
		})),
	);
	expect(descriptors.size).toBe(3);
	for (const variant of variants) {
		expect(
			descriptors.get(acpSessionKey(variant.args, variant.result)),
		).toEqual({
			displayName: variant.result.harness_display_name,
			prompt: variant.args.prompt,
		});
	}
	expect(
		acpSessionKey(
			{ harness_id: "a", working_directory: "/b:c", session_id: "d" },
			undefined,
		),
	).not.toBe(
		acpSessionKey(
			{ harness_id: "a", working_directory: "/b", session_id: "c:d" },
			undefined,
		),
	);
	expect(
		acpSessionKey({ session_id: MockACPSession.session_id }, undefined),
	).toBe("");
});
