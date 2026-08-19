import { describe, expect, it } from "vitest";
import { getExecuteRenderData, shouldRenderTool } from "./toolVisibility";

const stoppedWorkspaceError =
	"workspace has no running agent: the workspace is likely stopped. Use the start_workspace tool to start it";

describe("toolVisibility", () => {
	describe("getExecuteRenderData", () => {
		it("parses execute output from result payloads", () => {
			expect(
				getExecuteRenderData(
					{ command: "git fetch origin" },
					{
						output: " fetched ",
						wall_duration_ms: "47200",
						background_process_id: "process-1",
						backgrounded: true,
					},
				),
			).toEqual({
				command: "git fetch origin",
				transcriptBlocks: [{ kind: "output", text: "fetched" }],
				errorText: "",
				durationMs: 47200,
				isBackgrounded: true,
			});
		});

		it("does not treat a foreground timeout's process ID as backgrounded", () => {
			// Foreground commands that exceed their timeout also return
			// background_process_id so the caller can re-attach; only an
			// explicit backgrounded flag marks an intentional launch.
			expect(
				getExecuteRenderData(
					{ command: "make test" },
					{
						success: false,
						error: "command timed out after 10s",
						exit_code: -1,
						background_process_id: "process-1",
					},
				).isBackgrounded,
			).toBe(false);
		});

		it("reads legacy background launches from the call args", () => {
			// Transcripts recorded before the backgrounded flag existed
			// carry the launch intent in the persisted args.
			expect(
				getExecuteRenderData(
					{ command: "npm start", run_in_background: true },
					{
						success: true,
						background_process_id: "process-1",
					},
				).isBackgrounded,
			).toBe(true);
		});

		it("does not let legacy args override an explicit negative result", () => {
			// A new-backend foreground timeout has backgrounded omitted
			// (not false), so the args fallback must not resurrect it.
			expect(
				getExecuteRenderData(
					{ command: "make test", run_in_background: true },
					{
						success: false,
						error: "command timed out after 10s",
						background_process_id: "process-1",
						backgrounded: false,
					},
				).isBackgrounded,
			).toBe(false);
		});

		it("recognizes legacy trailing-ampersand background launches", () => {
			// The execute tool promotes `cmd &` to background mode and
			// strips the ampersand, but the persisted args keep the
			// original command without run_in_background.
			expect(
				getExecuteRenderData(
					{ command: "npm start &" },
					{
						success: true,
						background_process_id: "process-1",
					},
				).isBackgrounded,
			).toBe(true);
		});

		it("ignores ampersand chains that are not background promotions", () => {
			expect(
				getExecuteRenderData(
					{ command: "cmd1 && cmd2" },
					{ success: true, background_process_id: "process-1" },
				).isBackgrounded,
			).toBe(false);
			expect(
				getExecuteRenderData(
					{ command: "cmd |& tee log" },
					{ success: true, background_process_id: "process-1" },
				).isBackgrounded,
			).toBe(false);
		});

		it("normalizes execute error results into transcript blocks", () => {
			const data = getExecuteRenderData(
				{ command: "ls -la" },
				{ error: stoppedWorkspaceError },
			);

			expect(data.command).toBe("ls -la");
			expect(data.transcriptBlocks).toEqual([
				{ kind: "error", text: stoppedWorkspaceError },
			]);
			expect(
				data.transcriptBlocks.map((block) => block.text).join("\n"),
			).toContain("workspace has no running agent");
		});

		it("keeps output before error when both fields exist", () => {
			expect(
				getExecuteRenderData(
					{ command: "make build" },
					{ output: " compiling ", error: " failed " },
				).transcriptBlocks,
			).toEqual([
				{ kind: "output", text: "compiling" },
				{ kind: "error", text: "failed" },
			]);
		});

		it("uses message as an error fallback when error is blank", () => {
			expect(
				getExecuteRenderData(
					{ command: "coder login" },
					{ error: "  ", message: " auth required " },
				).transcriptBlocks,
			).toEqual([{ kind: "error", text: "auth required" }]);
		});
	});

	describe("shouldRenderTool", () => {
		it("hides execute rows without a command", () => {
			expect(
				shouldRenderTool({
					name: "execute",
					status: "completed",
					args: {},
					result: { output: "ignored" },
				}),
			).toBe(false);
		});

		it("hides running wait_agent rows until chat_id is available", () => {
			expect(
				shouldRenderTool({
					name: "wait_agent",
					status: "running",
					args: {},
					result: { status: "pending" },
				}),
			).toBe(false);
		});

		it("hides running message_agent rows until chat_id is available", () => {
			expect(
				shouldRenderTool({
					name: "message_agent",
					status: "running",
					args: { message: "continue" },
					result: { status: "pending" },
				}),
			).toBe(false);
		});

		it("hides running close_agent (legacy alias) rows until chat_id is available", () => {
			expect(
				shouldRenderTool({
					name: "close_agent",
					status: "running",
					args: {},
					result: { status: "running" },
				}),
			).toBe(false);
		});

		it("hides running interrupt_agent rows until chat_id is available", () => {
			expect(
				shouldRenderTool({
					name: "interrupt_agent",
					status: "running",
					args: {},
					result: { status: "running" },
				}),
			).toBe(false);
		});

		it("renders list_agents rows regardless of chat_id", () => {
			expect(
				shouldRenderTool({
					name: "list_agents",
					status: "running",
					args: {},
					result: undefined,
				}),
			).toBe(true);
		});

		it("renders running lifecycle rows once args provide the chat_id", () => {
			expect(
				shouldRenderTool({
					name: "wait_agent",
					status: "running",
					args: { chat_id: "child-chat-1" },
					result: { status: "pending" },
				}),
			).toBe(true);
		});

		it("renders completed close_agent (legacy alias) rows even if chat_id is absent", () => {
			expect(
				shouldRenderTool({
					name: "close_agent",
					status: "completed",
					args: {},
					result: { status: "completed" },
				}),
			).toBe(true);
		});

		it("keeps unrelated tools visible", () => {
			expect(
				shouldRenderTool({
					name: "read_file",
					status: "completed",
					args: { path: "README.md" },
					result: { content: "docs" },
				}),
			).toBe(true);
		});
	});
});
