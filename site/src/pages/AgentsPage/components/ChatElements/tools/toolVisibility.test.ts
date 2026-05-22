import { describe, expect, it } from "vitest";
import { getExecuteRenderData, shouldRenderTool } from "./toolVisibility";

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
				output: "fetched",
				durationMs: 47200,
				isBackgrounded: true,
				authenticateURL: "https://example.com/auth",
				providerLabel: "GitHub",
			});
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

		it("hides running close_agent rows until chat_id is available", () => {
			expect(
				shouldRenderTool({
					name: "close_agent",
					status: "running",
					args: {},
					result: { status: "running" },
				}),
			).toBe(false);
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

		it("renders completed lifecycle rows even if chat_id is absent", () => {
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
