import { describe, expect, it } from "vitest";
import { getExecuteRenderData, shouldRenderTool } from "./toolVisibility";

const stoppedWorkspaceError =
	"workspace has no running agent: the workspace is likely stopped. Use the start_workspace tool to start it";

describe("toolVisibility", () => {
	describe("getExecuteRenderData", () => {
		it("parses execute output and auth metadata from result payloads", () => {
			expect(
				getExecuteRenderData(
					{ command: "git fetch origin" },
					{
						output: " fetched ",
						wall_duration_ms: "47200",
						background_process_id: "process-1",
						auth_required: true,
						authenticate_url: "https://example.com/auth",
						provider_display_name: "GitHub",
					},
				),
			).toEqual({
				command: "git fetch origin",
				transcriptBlocks: [{ kind: "output", text: "fetched" }],
				durationMs: 47200,
				isBackgrounded: true,
				authenticateURL: "https://example.com/auth",
				providerLabel: "GitHub",
			});
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
		it("hides execute rows with neither a command nor an auth prompt", () => {
			expect(
				shouldRenderTool({
					name: "execute",
					status: "completed",
					args: {},
					result: { output: "ignored" },
				}),
			).toBe(false);
		});

		it("keeps execute rows when auth is required even without a command", () => {
			expect(
				shouldRenderTool({
					name: "execute",
					status: "completed",
					args: {},
					result: {
						auth_required: true,
						authenticate_url: "https://example.com/auth",
					},
				}),
			).toBe(true);
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

		it("renders list_agents rows even without a chat_id", () => {
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
