import { describe, expect, test, beforeEach } from "bun:test";
import { CoderTaskAction } from "./action";
import type { Octokit } from "./action";
import {
	MockCoderClient,
	createMockOctokit,
	createMockInputs,
	mockUser,
	mockTask,
	mockTemplate,
} from "./test-helpers";

describe("CoderTaskAction", () => {
	let coderClient: MockCoderClient;
	let octokit: ReturnType<typeof createMockOctokit>;

	beforeEach(() => {
		coderClient = new MockCoderClient();
		octokit = createMockOctokit();
	});

	describe("parseGithubIssueUrl", () => {
		test("parses valid GitHub issue URL", () => {
			const inputs = createMockInputs({
				githubIssueURL: "https://github.com/owner/repo/issues/123",
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (
				action as unknown as CoderTaskAction
			).parseGithubIssueURL();

			expect(result).toEqual({
				githubOrg: "owner",
				githubRepo: "repo",
				githubIssueNumber: 123,
			});
		});

		test("throws when no issue URL provided", () => {
			const inputs = createMockInputs({ githubIssueURL: undefined });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (
				action as unknown as CoderTaskAction
			).parseGithubIssueURL();

			expect(result).toThrowError("Missing issue URL");
		});

		test("throws for invalid URL format", () => {
			const inputs = createMockInputs({ githubIssueURL: "not-a-url" });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (
				action as unknown as CoderTaskAction
			).parseGithubIssueURL();

			expect(result).toThrowError("Invalid issue URL: not-a-url");
		});

		test("handled non-github.com URL", () => {
			const inputs = createMockInputs({
				githubIssueURL: "https://code.acme.com/owner/repo/issues/123",
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (
				action as unknown as CoderTaskAction
			).parseGithubIssueURL();

			expect(result).toEqual({
				githubOrg: "owner",
				githubRepo: "repo",
				githubIssueNumber: 123,
			});
		});

		test("handles URL with trailing junk", () => {
			const inputs = createMockInputs({
				githubIssueURL:
					"https://github.com/owner/repo/issues/123/?param=value#anchor",
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			const result = (
				action as unknown as CoderTaskAction
			).parseGithubIssueURL();

			// Should still parse correctly
			expect(result).toEqual({
				githubOrg: "owner",
				githubRepo: "repo",
				githubIssueNumber: 123,
			});
		});
	});

	describe("generateTaskUrl", () => {
		test("generates correct task URL", () => {
			const inputs = createMockInputs();
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

		test("handles URL with trailing junk", () => {
			const inputs = createMockInputs({
				coderURL: "https://coder.test/?param=value#anchor",
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

	test("creates new task successfully", async () => {
		// Setup
		coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
		coderClient.mockGetTask.mockResolvedValue(null);
		coderClient.mockCreateTask.mockResolvedValue(mockTask);

		const inputs = createMockInputs({
			githubUserID: 12345,
		});
		const action = new CoderTaskAction(
			coderClient,
			octokit as unknown as Octokit,
			inputs,
		);

		// Execute
		const result = await action.run();

		// Verify
		expect(coderClient.mockGetCoderUserByGithubID).toHaveBeenCalledWith(12345);
		expect(coderClient.mockGetTask).toHaveBeenCalledWith(
			mockUser.username,
			mockTask.name,
		);
		expect(coderClient.mockCreateTask).toHaveBeenCalledWith({
			username: mockUser.username,
			name: mockTask.name,
			template_id: mockTemplate.id,
			input: "idk",
		});
		expect(result.coderUsername).toBe("testuser");
		expect(result.taskCreated).toBe(false);
		expect(result.taskUrl).toContain("/tasks/testuser/");
	});

	test("sends prompt to existing task", async () => {
		// Setup
		coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
		coderClient.mockGetTemplateByOrganizationAndName.mockResolvedValue(
			mockTemplate,
		);
		coderClient.mockGetTemplateVersionPresets.mockResolvedValue([]);
		coderClient.mockGetTask.mockResolvedValue(mockTask);
		coderClient.mockSendTaskInput.mockResolvedValue(undefined);

		const inputs = createMockInputs({
			githubUserID: 12345,
		});
		const action = new CoderTaskAction(
			coderClient,
			octokit as unknown as Octokit,
			inputs,
		);

		// Execute
		const result = await action.run();

		// Verify
		expect(coderClient.mockGetTask).toHaveBeenCalledWith(
			mockUser.username,
			mockTask.name,
		);
		expect(coderClient.mockSendTaskInput).toHaveBeenCalledWith(mockTask.id, {
			prompt: "test prompt",
		});
		expect(coderClient.mockCreateTask).not.toHaveBeenCalled();
		expect(result.taskCreated).toBe(false);
	});

	test("errors without issue URL", async () => {
		// Setup
		coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
		coderClient.mockGetTask.mockResolvedValue(null);
		coderClient.mockCreateTask.mockResolvedValue(mockTask);

		const inputs = createMockInputs({
			githubUserID: 12345,
			githubIssueURL: undefined,
		});
		const action = new CoderTaskAction(
			coderClient,
			octokit as unknown as Octokit,
			inputs,
		);

		// Execute
		expect(action.run()).rejects.toThrowError("Missing issue URL");
	});

	test("comments on issue", async () => {
		// Setup
		coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
		coderClient.mockGetTask.mockResolvedValue(null);
		coderClient.mockGetTemplateByOrganizationAndName.mockResolvedValue(
			mockTemplate,
		);
		coderClient.mockGetTemplateVersionPresets.mockResolvedValue([]);
		coderClient.mockCreateTask.mockResolvedValue(mockTask);
		octokit.rest.issues.listComments.mockResolvedValue({
			data: [],
		} as ReturnType<typeof octokit.rest.issues.listComments>);
		octokit.rest.issues.createComment.mockResolvedValue(
			{} as ReturnType<typeof octokit.rest.issues.updateComment>,
		);

		const inputs = createMockInputs({
			githubUserID: 12345,
			githubIssueURL: "https://github.com/owner/repo/issues/123",
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
				body: "Task created: https://coder.test/tasks/testuser/task-123",
			}),
		);
	});

	test("updates existing comment on issue", async () => {
		// Setup
		coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
		coderClient.mockGetTask.mockResolvedValue(null);
		coderClient.mockGetTemplateByOrganizationAndName.mockResolvedValue(
			mockTemplate,
		);
		coderClient.mockGetTemplateVersionPresets.mockResolvedValue([]);
		coderClient.mockCreateTask.mockResolvedValue(mockTask);
		octokit.rest.issues.listComments.mockResolvedValue({
			data: [
				{
					id: 23455,
					body: "An unrelated comment",
				},
				{
					id: 23456,
					body: "Task created:",
				},
				{
					id: 23457,
					body: "Another unrelated comment",
				},
			],
		} as ReturnType<typeof octokit.rest.issues.listComments>);
		octokit.rest.issues.updateComment.mockResolvedValue(
			{} as ReturnType<typeof octokit.rest.issues.updateComment>,
		);

		const inputs = createMockInputs({
			githubUserID: 12345,
			githubIssueURL: "https://github.com/owner/repo/issues/123",
		});
		const action = new CoderTaskAction(
			coderClient,
			octokit as unknown as Octokit,
			inputs,
		);

		// Execute
		await action.run();

		// Verify
		expect(octokit.rest.issues.updateComment).toHaveBeenCalledWith(
			expect.objectContaining({
				owner: "owner",
				repo: "repo",
				comment_id: 23456,
				body: "Task created: https://coder.test/tasks/testuser/task-123",
			}),
		);
	});

	test("handles error when comment on issue fails", async () => {
		// Setup
		coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
		coderClient.mockGetTask.mockResolvedValue(null);
		coderClient.mockGetTemplateByOrganizationAndName.mockResolvedValue(
			mockTemplate,
		);
		coderClient.mockGetTemplateVersionPresets.mockResolvedValue([]);
		coderClient.mockCreateTask.mockResolvedValue(mockTask);
		octokit.rest.issues.listComments.mockResolvedValue({
			data: [],
		} as ReturnType<typeof octokit.rest.issues.listComments>);
		octokit.rest.issues.createComment.mockRejectedValue(
			new Error("Failed to comment on issue"),
		);

		const inputs = createMockInputs({
			githubUserID: 12345,
			githubIssueURL: "https://github.com/owner/repo/issues/123",
		});
		const action = new CoderTaskAction(
			coderClient,
			octokit as unknown as Octokit,
			inputs,
		);

		await action.run();
		expect(octokit.rest.issues.createComment).toHaveBeenCalledWith(
			expect.objectContaining({
				owner: "owner",
				repo: "repo",
				issue_number: 123,
			}),
		);
	});

	describe("run - Error Scenarios", () => {
		test("throws error when Coder user not found", async () => {
			coderClient.mockGetCoderUserByGithubID.mockRejectedValue(
				new Error("No Coder user found with GitHub user ID 12345"),
			);

			const inputs = createMockInputs({ githubUserID: 12345 });
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
			coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
			coderClient.mockGetTask.mockResolvedValue(null);
			coderClient.mockGetTemplateByOrganizationAndName.mockRejectedValue(
				new Error("Template not found"),
			);
			coderClient.mockCreateTask.mockRejectedValue(
				new Error("Template not found: nonexistent"),
			);

			const inputs = createMockInputs({
				githubUserID: 12345,
				coderTemplateName: "nonexistent",
			});
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			expect(action.run()).rejects.toThrow("Template not found");
		});

		test("throws error when task creation fails", async () => {
			coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
			coderClient.mockGetTask.mockResolvedValue(null);
			coderClient.mockGetTemplateByOrganizationAndName.mockResolvedValue(
				mockTemplate,
			);
			coderClient.mockGetTemplateVersionPresets.mockResolvedValue([]);
			coderClient.mockCreateTask.mockRejectedValue(
				new Error("Failed to create task"),
			);

			const inputs = createMockInputs({ githubUserID: 12345 });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			expect(action.run()).rejects.toThrow("Failed to create task");
		});

		test("throws error on permission denied", async () => {
			coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
			coderClient.mockGetTask.mockResolvedValue(null);
			coderClient.mockGetTemplateByOrganizationAndName.mockResolvedValue(
				mockTemplate,
			);
			coderClient.mockGetTemplateVersionPresets.mockResolvedValue([]);
			coderClient.mockCreateTask.mockRejectedValue(
				new Error("Permission denied"),
			);

			const inputs = createMockInputs({ githubUserID: 12345 });
			const action = new CoderTaskAction(
				coderClient,
				octokit as unknown as Octokit,
				inputs,
			);

			expect(action.run()).rejects.toThrow("Permission denied");
		});
	});

	// NOTE: this may or may not work in the real world depending on the permissions of the user
	test("handles cross-repository issue", async () => {
		// Setup
		coderClient.mockGetCoderUserByGithubID.mockResolvedValue(mockUser);
		coderClient.mockGetTask.mockResolvedValue(null);
		coderClient.mockGetTemplateByOrganizationAndName.mockResolvedValue(
			mockTemplate,
		);
		coderClient.mockGetTemplateVersionPresets.mockResolvedValue([]);
		coderClient.mockCreateTask.mockResolvedValue(mockTask);
		octokit.rest.issues.listComments.mockResolvedValue({
			data: [],
		} as ReturnType<typeof octokit.rest.issues.listComments>);
		octokit.rest.issues.createComment.mockResolvedValue(
			{} as ReturnType<typeof octokit.rest.issues.createComment>,
		);

		const inputs = createMockInputs({
			githubIssueURL:
				"https://github.com/different-owner/different-repo/issues/456",
		});
		const action = new CoderTaskAction(
			coderClient,
			octokit as unknown as Octokit,
			inputs,
		);

		await action.run();
		expect(octokit.rest.issues.createComment).toHaveBeenCalledWith(
			expect.objectContaining({
				owner: "different-owner",
				repo: "different-repo",
				issue_number: 456,
			}),
		);
	});
});
