import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { expect, it, vi } from "vitest";
import {
	MockACPSession,
	MockACPSpawnArgs,
	MockACPWaitingResult,
	MockACPWaitResult,
} from "#/testHelpers/acp";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { ACPContext, buildACPSessionDescriptors } from "./ACPContext";
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
		args: { session_id: MockACPSession.session_id },
		isError: false,
	} as const;
	const queryClient = createTestQueryClient();
	const activity = (result: unknown) => (
		<QueryClientProvider client={queryClient}>
			<ACPTool {...props} result={result} />
		</QueryClientProvider>
	);
	const { rerender } = renderComponent(activity(MockACPWaitingResult));
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
	rerender(activity(MockACPWaitResult));
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
	const renamedSession = {
		...MockACPSession,
		harness_display_name: "Workspace Assistant",
	};
	const waitResult = { ...MockACPWaitResult, ...renamedSession };
	const sessions = buildACPSessionDescriptors([
		{
			id: "spawn",
			name: ACPToolNames.SpawnAgent,
			status: "completed",
			isError: false,
			args: MockACPSpawnArgs,
			result: renamedSession,
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
					args={{ session_id: MockACPSession.session_id }}
					result={result}
					isError={false}
				/>
			</ACPContext>
		</QueryClientProvider>
	);
	const { rerender } = renderComponent(activity(waitResult));
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
	rerender(activity({ ...waitResult, timed_out: true }));
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
				args: { session_id: MockACPSession.session_id, message: "A new task" },
				result: { session_id: MockACPSession.session_id },
			},
		]).get(MockACPSession.session_id),
	).toEqual({
		displayName: MockACPSession.harness_display_name,
		prompt: MockACPSpawnArgs.prompt,
	});
});

it("uses listed harnesses when a spawn call is outside the loaded history", () => {
	expect(
		buildACPSessionDescriptors([
			{
				id: "list",
				name: ACPToolNames.ListAgents,
				status: "completed",
				isError: false,
				result: { agents: [MockACPSession] },
			},
			{
				id: "wait",
				name: ACPToolNames.WaitAgent,
				status: "running",
				isError: false,
				args: { session_id: MockACPSession.session_id },
				result: { output: "Checking tests" },
			},
		]).get(MockACPSession.session_id),
	).toEqual({ displayName: MockACPSession.harness_display_name, prompt: "" });
});
