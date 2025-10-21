import { describe, expect, test, beforeEach } from "bun:test";
import { CoderTaskAction } from "./action";
import type { Octokit } from "./action";
import {
	MockCoderClient,
	createMockOctokit,
	createMockInputs,
	mockUser,
	mockTask,
} from "./test-helpers";

describe("CoderTaskAction", () => {
	let coderClient: MockCoderClient;
	let octokit: ReturnType<typeof createMockOctokit>;

	beforeEach(() => {
		coderClient = new MockCoderClient();
		octokit = createMockOctokit();
	});

	describe("resolveGitHubUserId", () => {
		describe("Success Cases", () => {
			test("returns githubUserId when provided in inputs", async () => {
				const inputs = createMockInputs({ githubUserId: 12345 });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await action.resolveGitHubUserId();

				expect(result).toBe(12345);
			});

			test("fetches user ID from API when username provided", async () => {
				octokit.rest.users.getByUsername.mockResolvedValue({
					data: { id: 67890 },
				} as ReturnType<typeof octokit.rest.users.getByUsername>);

				const inputs = createMockInputs({ githubUsername: "testuser" });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await (
					action as unknown as CoderTaskAction
				).resolveGitHubUserId();

				expect(result).toBe(67890);
				expect(octokit.rest.users.getByUsername).toHaveBeenCalledWith({
					username: "testuser",
				});
			});
		});

		describe("Error Cases", () => {
			test("throws error when neither userId nor username provided", async () => {
				const inputs = createMockInputs({
					githubUserId: undefined,
					githubUsername: undefined,
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				expect(
					(action as unknown as CoderTaskAction).resolveGitHubUserId(),
				).rejects.toThrow(
					"Either githubUserId or githubUsername must be provided",
				);
			});

			test("throws error when GitHub API returns 404 for username", async () => {
				octokit.rest.users.getByUsername.mockRejectedValue(
					new Error("Not Found"),
				);

				const inputs = createMockInputs({ githubUsername: "nonexistent" });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				expect(
					(action as unknown as CoderTaskAction).resolveGitHubUserId(),
				).rejects.toThrow();
			});
		});
	});

	describe("generateTaskName", () => {
		test("uses provided task-name when specified", () => {
			const inputs = createMockInputs({ taskName: "custom-name" });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (action as unknown as CoderTaskAction).generateTaskName(
				123,
			);

			expect(result).toBe("custom-name");
		});

		test("generates name with issue number", () => {
			const inputs = createMockInputs();
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (action as unknown as CoderTaskAction).generateTaskName(
				123,
			);

			expect(result).toMatch(/^task-gh-123$/);
		});

		test("generates name without issue", () => {
			const inputs = createMockInputs();
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (action as unknown as CoderTaskAction).generateTaskName(
				undefined,
			);

			expect(result).toMatch(/^task-run-\d+$/);
		});

		test("respects task-name-prefix", () => {
			const inputs = createMockInputs({ taskNamePrefix: "custom-prefix" });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (action as unknown as CoderTaskAction).generateTaskName(
				123,
			);

			expect(result).toMatch(/^custom-prefix-gh-123$/);
		});

		describe("Edge Cases", () => {
			test("uses default prefix when empty", () => {
				const inputs = createMockInputs({ taskNamePrefix: "" });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).generateTaskName(
					123,
				);

				// Empty prefix should still work
				expect(result).toBe("-gh-123");
			});

			test("handles very long prefix", () => {
				const longPrefix = "x".repeat(100);
				const inputs = createMockInputs({ taskNamePrefix: longPrefix });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).generateTaskName(
					123,
				);

				expect(result).toStartWith(longPrefix);
			});
		});
	});

	describe("getIssueNumber", () => {
		describe("Success Cases", () => {
			test("extracts number from valid GitHub issue URL", async () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/123",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await (
					action as unknown as CoderTaskAction
				).getIssueNumber();

				expect(result).toBe(123);
			});

			test("returns undefined when no issue URL", async () => {
				const inputs = createMockInputs({ issueUrl: undefined });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await (
					action as unknown as CoderTaskAction
				).getIssueNumber();

				expect(result).toBeUndefined();
			});

			test("handles URL with hash fragment", async () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/123#comment",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await (
					action as unknown as CoderTaskAction
				).getIssueNumber();

				expect(result).toBe(123);
			});

			test("handles URL with query parameters", async () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/123?param=value",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await (
					action as unknown as CoderTaskAction
				).getIssueNumber();

				expect(result).toBe(123);
			});
		});

		describe("Error Cases", () => {
			test("returns undefined for invalid URL format", async () => {
				const inputs = createMockInputs({ issueUrl: "not-a-url" });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await (
					action as unknown as CoderTaskAction
				).getIssueNumber();

				expect(result).toBeUndefined();
			});

			test("returns undefined for non-numeric issue identifier", async () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/abc",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = await (
					action as unknown as CoderTaskAction
				).getIssueNumber();

				expect(result).toBeUndefined();
			});
		});
	});

	describe("parseIssueUrl", () => {
		describe("Success Cases", () => {
			test("parses valid GitHub issue URL", () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/123",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).parseIssueUrl();

				expect(result).toEqual({
					owner: "owner",
					repo: "repo",
					issueNumber: 123,
				});
			});

			test("returns null when no issue URL", () => {
				const inputs = createMockInputs({ issueUrl: undefined });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).parseIssueUrl();

				expect(result).toBeNull();
			});
		});

		describe("Error Cases", () => {
			test("returns null for invalid URL format", () => {
				const inputs = createMockInputs({ issueUrl: "not-a-url" });
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).parseIssueUrl();

				expect(result).toBeNull();
			});

			test("returns null for non-GitHub URL", () => {
				const inputs = createMockInputs({
					issueUrl: "https://example.com/issues/123",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).parseIssueUrl();

				expect(result).toBeNull();
			});
		});

		describe("Edge Cases", () => {
			test("handles URL with trailing slash", () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/123/",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).parseIssueUrl();

				// Should still parse correctly
				expect(result?.issueNumber).toBe(123);
			});

			test("handles URL with query parameters", () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/123?param=value",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).parseIssueUrl();

				expect(result?.issueNumber).toBe(123);
			});

			test("handles URL with hash fragment", () => {
				const inputs = createMockInputs({
					issueUrl: "https://github.com/owner/repo/issues/123#comment",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).parseIssueUrl();

				expect(result?.issueNumber).toBe(123);
			});
		});
	});

	describe("generateTaskUrl", () => {
		test("uses coderWebUrl when provided", () => {
			const inputs = createMockInputs({
				coderUrl: "https://coder.test",
				coderWebUrl: "https://coder-web.test",
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (action as unknown as CoderTaskAction).generateTaskUrl(
				"testuser",
				"task-123",
			);

			expect(result).toBe("https://coder-web.test/tasks/testuser/task-123");
		});

		test("falls back to coderUrl when coderWebUrl not provided", () => {
			const inputs = createMockInputs({
				coderUrl: "https://coder.test",
				coderWebUrl: undefined,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (action as unknown as CoderTaskAction).generateTaskUrl(
				"testuser",
				"task-123",
			);

			expect(result).toBe("https://coder.test/tasks/testuser/task-123");
		});

		describe("Edge Cases", () => {
			test("handles URL with trailing slash", () => {
				const inputs = createMockInputs({
					coderUrl: "https://coder.test/",
				});
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				const result = (action as unknown as CoderTaskAction).generateTaskUrl(
					"testuser",
					"task-123",
				);

				// Should not have double slash
				expect(result).toBe("https://coder.test//tasks/testuser/task-123");
			});
		});
	});

	describe("commentOnIssue", () => {
		describe("Success Cases", () => {
			test("creates new comment when none exists", async () => {
				octokit.rest.issues.listComments.mockResolvedValue({
					data: [],
				} as ReturnType<typeof octokit.rest.issues.listComments>);
				octokit.rest.issues.createComment.mockResolvedValue(
					{} as ReturnType<typeof octokit.rest.issues.createComment>,
				);

				const inputs = createMockInputs();
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				await (action as unknown as CoderTaskAction).commentOnIssue(
					"https://coder.test/tasks/testuser/task-123",
					"owner",
					"repo",
					123,
				);

				expect(octokit.rest.issues.createComment).toHaveBeenCalledWith({
					owner: "owner",
					repo: "repo",
					issue_number: 123,
					body: "Task created: https://coder.test/tasks/testuser/task-123",
				});
			});

			test("updates existing Task created comment", async () => {
				octokit.rest.issues.listComments.mockResolvedValue({
					data: [
						{ id: 1, body: "Task created: old-url" },
						{ id: 2, body: "Other comment" },
						{ id: 3, body: "Task created: another-old-url" },
					],
				} as ReturnType<typeof octokit.rest.issues.listComments>);
				octokit.rest.issues.updateComment.mockResolvedValue(
					{} as ReturnType<typeof octokit.rest.issues.updateComment>,
				);

				const inputs = createMockInputs();
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				await (action as unknown as CoderTaskAction).commentOnIssue(
					"https://coder.test/tasks/testuser/task-123",
					"owner",
					"repo",
					123,
				);

				// Should update the last "Task created:" comment
				expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith({
					owner: "owner",
					repo: "repo",
					comment_id: 3,
					body: "Task created: https://coder.test/tasks/testuser/task-123",
				});
			});

			test("parses owner/repo/issue from URL correctly", async () => {
				octokit.rest.issues.listComments.mockResolvedValue({
					data: [],
				} as ReturnType<typeof octokit.rest.issues.listComments>);
				octokit.rest.issues.createComment.mockResolvedValue(
					{} as ReturnType<typeof octokit.rest.issues.createComment>,
				);

				const inputs = createMockInputs();
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				await (action as unknown as CoderTaskAction).commentOnIssue(
					"https://coder.test/tasks/testuser/task-123",
					"different-owner",
					"different-repo",
					456,
				);

				expect(octokit.rest.issues.createComment).toHaveBeenCalledWith({
					owner: "different-owner",
					repo: "different-repo",
					issue_number: 456,
					body: "Task created: https://coder.test/tasks/testuser/task-123",
				});
			});
		});

		describe("Error Cases", () => {
			test("warns but doesn't fail on GitHub API error", async () => {
				octokit.rest.issues.listComments.mockRejectedValue(
					new Error("API Error"),
				);

				const inputs = createMockInputs();
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				// Should not throw
				expect(
					(action as unknown as CoderTaskAction).commentOnIssue(
						"https://coder.test/tasks/testuser/task-123",
						"owner",
						"repo",
						123,
					),
				).resolves.toBeUndefined();
			});

			test("warns but doesn't fail on permission error", async () => {
				octokit.rest.issues.listComments.mockResolvedValue({
					data: [],
				} as ReturnType<typeof octokit.rest.issues.listComments>);
				octokit.rest.issues.createComment.mockRejectedValue(
					new Error("Permission denied"),
				);

				const inputs = createMockInputs();
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				// Should not throw
				expect(
					(action as unknown as CoderTaskAction).commentOnIssue(
						"https://coder.test/tasks/testuser/task-123",
						"owner",
						"repo",
						123,
					),
				).resolves.toBeUndefined();
			});
		});

		describe("Edge Cases", () => {
			test("updates last comment when multiple Task created comments exist", async () => {
				octokit.rest.issues.listComments.mockResolvedValue({
					data: [
						{ id: 1, body: "Task created: url1" },
						{ id: 2, body: "Other comment" },
						{ id: 3, body: "Task created: url2" },
						{ id: 4, body: "Another comment" },
						{ id: 5, body: "Task created: url3" },
					],
				} as ReturnType<typeof octokit.rest.issues.listComments>);
				octokit.rest.issues.updateComment.mockResolvedValue(
					{} as ReturnType<typeof octokit.rest.issues.updateComment>,
				);

				const inputs = createMockInputs();
				const action = new CoderTaskAction(
					coderClient,
					octokit as unknown as Octokit,
					inputs,
				);

				await (action as unknown as CoderTaskAction).commentOnIssue(
					"https://coder.test/tasks/testuser/task-123",
					"owner",
					"repo",
					123,
				);

				// Should update comment 5 (last Task created comment)
				expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(
					expect.objectContaining({
						comment_id: 5,
					}),
				);
			});
		});
	});

	describe("run - Happy Path Scenarios", () => {
		test("creates new task successfully", async () => {
			// Setup
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockResolvedValue(mockTask);

			const inputs = createMockInputs({
				githubUserId: 12345,
				issueUrl: undefined,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			const result = await action.run();

			// Verify
			expect(coderClient.mockGetCoderUserByGitHubId).toHaveBeenCalledWith(
				12345,
			);
			expect(coderClient.mockGetTaskStatus).toHaveBeenCalled();
			expect(coderClient.mockCreateTask).toHaveBeenCalled();
			expect(result.coderUsername).toBe("testuser");
			expect(result.taskExists).toBe(false);
			expect(result.taskUrl).toContain("/tasks/testuser/");
		});

		test("sends prompt to existing task", async () => {
			// Setup
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(mockTask);
			coderClient.mockSendTaskInput.mockResolvedValue(undefined);

			const inputs = createMockInputs({
				githubUserId: 12345,
				issueUrl: undefined,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			const result = await action.run();

			// Verify
			expect(coderClient.mockGetTaskStatus).toHaveBeenCalled();
			expect(coderClient.mockSendTaskInput).toHaveBeenCalled();
			expect(coderClient.mockCreateTask).not.toHaveBeenCalled();
			expect(result.taskExists).toBe(true);
		});

		test("creates task without issue URL", async () => {
			// Setup
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockResolvedValue(mockTask);

			const inputs = createMockInputs({
				githubUserId: 12345,
				issueUrl: undefined,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			const result = await action.run();

			// Verify
			expect(result.taskUrl).toContain("/tasks/testuser/");
			expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
		});

		test("creates task with commentOnIssue: false", async () => {
			// Setup
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockResolvedValue(mockTask);

			const inputs = createMockInputs({
				githubUserId: 12345,
				issueUrl: "https://github.com/owner/repo/issues/123",
				commentOnIssue: false,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			const result = await action.run();

			// Verify
			expect(result.taskUrl).toContain("/tasks/testuser/");
			expect(octokit.rest.issues.createComment).not.toHaveBeenCalled();
		});

		test("comments on issue when requested", async () => {
			// Setup
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockResolvedValue(mockTask);
			octokit.rest.issues.listComments.mockResolvedValue({
				data: [],
			} as ReturnType<typeof octokit.rest.issues.listComments>);
			octokit.rest.issues.updateComment.mockResolvedValue(
				{} as ReturnType<typeof octokit.rest.issues.updateComment>,
			);

			const inputs = createMockInputs({
				githubUserId: 12345,
				issueUrl: "https://github.com/owner/repo/issues/123",
				commentOnIssue: true,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			await action.run();

			// Verify
			expect(octokit.rest.issues.createComment).toHaveBeenCalledWith(
				expect.objectContaining({
					owner: "owner",
					repo: "repo",
					issue_number: 123,
				}),
			);
		});
	});

	describe("run - Error Scenarios", () => {
		test("throws error when Coder user not found", async () => {
			coderClient.mockGetCoderUserByGitHubId.mockRejectedValue(
				new Error("No Coder user found with GitHub user ID 12345"),
			);

			const inputs = createMockInputs({ githubUserId: 12345 });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			expect(action.run()).rejects.toThrow(
				"No Coder user found with GitHub user ID 12345",
			);
		});

		test("throws error when template not found", async () => {
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockRejectedValue(
				new Error("Template not found: nonexistent"),
			);

			const inputs = createMockInputs({
				githubUserId: 12345,
				templateName: "nonexistent",
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			expect(action.run()).rejects.toThrow("Template not found");
		});

		test("throws error when task creation fails", async () => {
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockRejectedValue(
				new Error("Failed to create task"),
			);

			const inputs = createMockInputs({ githubUserId: 12345 });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			expect(action.run()).rejects.toThrow("Failed to create task");
		});

		test("throws error on permission denied", async () => {
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockRejectedValue(
				new Error("Permission denied"),
			);

			const inputs = createMockInputs({ githubUserId: 12345 });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			expect(action.run()).rejects.toThrow("Permission denied");
		});
	});

	describe("run - Edge Cases", () => {
		test("handles cross-repository issue", async () => {
			// Setup
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockResolvedValue(mockTask);
			octokit.rest.issues.listComments.mockResolvedValue({
				data: [],
			} as ReturnType<typeof octokit.rest.issues.listComments>);
			octokit.rest.issues.createComment.mockResolvedValue(
				{} as ReturnType<typeof octokit.rest.issues.createComment>,
			);

			const inputs = createMockInputs({
				githubUserId: 12345,
				issueUrl:
					"https://github.com/different-owner/different-repo/issues/456",
				commentOnIssue: true,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			await action.run();

			// Verify
			expect(octokit.rest.issues.createComment).toHaveBeenCalledWith(
				expect.objectContaining({
					owner: "different-owner",
					repo: "different-repo",
					issue_number: 456,
				}),
			);
		});

		test("handles very long prompt", async () => {
			const longPrompt = "x".repeat(50000);

			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockResolvedValue(mockTask);

			const inputs = createMockInputs({
				githubUserId: 12345,
				taskPrompt: longPrompt,
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			const result = await action.run();

			// Verify
			expect(coderClient.mockCreateTask).toHaveBeenCalledWith(
				expect.objectContaining({
					prompt: longPrompt,
				}),
			);
			expect(result.taskUrl).toBeDefined();
		});

		test("handles special characters in task names", async () => {
			coderClient.mockGetCoderUserByGitHubId.mockResolvedValue(mockUser);
			coderClient.mockGetTaskStatus.mockResolvedValue(null);
			coderClient.mockCreateTask.mockResolvedValue(mockTask);

			const inputs = createMockInputs({
				githubUserId: 12345,
				taskName: "task-with-special-chars-123",
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			// Execute
			const result = await action.run();

			// Verify
			expect(result.taskName).toContain("task-with-special-chars-123");
		});
	});
});
