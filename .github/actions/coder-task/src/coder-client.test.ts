import { describe, expect, test, beforeEach, mock } from "bun:test";
import { CoderClient, CoderAPIError } from "./coder-client";
import {
	mockUser,
	mockUserList,
	mockUserListEmpty,
	mockUserListDuplicate,
	mockTemplate,
	mockTask,
	mockTaskList,
	mockTaskListEmpty,
	createMockResponse,
} from "./test-helpers";

describe("CoderClient", () => {
	let client: CoderClient;
	let mockFetch: ReturnType<typeof mock>;

	beforeEach(() => {
		client = new CoderClient("https://coder.test", "test-token");
		mockFetch = mock(() => Promise.resolve(createMockResponse([])));
		global.fetch = mockFetch as any;
	});

	describe("getCoderUserByGitHubId", () => {
		test("returns the user when found", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockUserList));
			const result = await client.getCoderUserByGitHubId(12345);
			expect(mockFetch).toHaveBeenCalledWith(
				"https://coder.test/api/v2/users?q=github_com_user_id%3A12345",
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
			expect(result.id).toBe(mockUser.id);
			expect(result.username).toBe(mockUser.username);
			expect(result.github_com_user_id).toBe(mockUser.github_com_user_id);
		});

		test("throws an error if multiple Coder users are found with the same GitHub ID", async () => {
			const secondUser = { ...mockUser, id: "different-id" };
			mockFetch.mockResolvedValue(createMockResponse(mockUserListDuplicate));
			expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
				CoderAPIError,
			);
			expect(mockFetch).toHaveBeenCalledWith(
				"https://coder.test/api/v2/users?q=github_com_user_id%3A12345",
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
		});

		test("throws an error if no Coder user is found with the given GitHub ID", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockUserListEmpty));
			expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
				CoderAPIError,
			);
			expect(mockFetch).toHaveBeenCalledWith(
				"https://coder.test/api/v2/users?q=github_com_user_id%3A12345",
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
		});

		test("throws error on 401 unauthorized", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Unauthorized" },
					{ ok: false, status: 401, statusText: "Unauthorized" },
				),
			);
			expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
				CoderAPIError,
			);
		});

		test("throws error on 500 server error", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Internal Server Error" },
					{ ok: false, status: 500, statusText: "Internal Server Error" },
				),
			);
			expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
				CoderAPIError,
			);
		});

		test("throws an error when GitHub user ID is 0", async () => {
			mockFetch.mockResolvedValue(createMockResponse([mockUser]));
			expect(client.getCoderUserByGitHubId(0)).rejects.toThrow(
				"GitHub user ID cannot be 0",
			);
		});
	});

	describe("getTemplateByOrganizationAndName", () => {
		test("the given template is returned successfully if it exists", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockTemplate));
			const result = await client.getTemplateByOrganizationAndName(
				"my org",
				"my template",
			);
			expect(mockFetch).toHaveBeenCalledWith(
				"https://coder.test/api/v2/organizations/my%20org/templates/my%20template",
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
			expect(result.id).toBe(mockTemplate.id);
			expect(result.name).toBe(mockTemplate.name);
			expect(result.active_version_id).toBe(mockTemplate.active_version_id);
		});

		test("throws an error when the given template is not found", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Not found" },
					{ ok: false, status: 404, statusText: "Not Found" },
				),
			);
			expect(
				client.getTemplateByOrganizationAndName("my-org", "nonexistent"),
			).rejects.toThrow(CoderAPIError);
		});
	});

	describe("getTask", () => {
		test("returns task when task exists", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockTaskList));
			const result = await client.getTask("testuser", "task-123");
			expect(result).not.toBeNull();
			expect(result?.id).toBe(mockTask.id);
			expect(result?.name).toBe(mockTask.name);
			expect(mockFetch).toHaveBeenCalledWith(
				"https://coder.test/api/experimental/tasks?q=owner%3Atestuser",
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
		});

		test("returns null when task doesn't exist (404)", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockTaskListEmpty));
			const result = await client.getTask("testuser", "task-123");
			expect(result).toBeNull();
			expect(mockFetch).toHaveBeenCalledWith(
				"https://coder.test/api/experimental/tasks?q=owner%3Atestuser",
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
		});
	});

	describe("createTask", () => {
		test("creates task successfully given valid input", async () => {
			mockFetch.mockResolvedValueOnce(createMockResponse(mockTemplate));
			mockFetch.mockResolvedValueOnce(createMockResponse(mockTask));
			const result = await client.createTask({
				name: "task-123",
				templateName: "my-template",
				templatePreset: "Default",
				organization: "coder",
				owner: "testuser",
				prompt: "Test prompt",
			});
			expect(result.id).toBe(mockTask.id);
			expect(result.name).toBe(mockTask.name);
			expect(mockFetch).toHaveBeenNthCalledWith(
				1,
				"https://coder.test/api/v2/organizations/coder/templates/my-template",
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
			expect(mockFetch).toHaveBeenNthCalledWith(
				2,
				"https://coder.test/api/experimental/tasks/testuser",
				expect.objectContaining({
					method: "POST",
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
					body: JSON.stringify({
						name: "task-123",
						template_id: mockTemplate.id,
						template_version_preset_id: "Default",
						prompt: "Test prompt",
					}),
				}),
			);
		});

		test("throws error when template not found", async () => {
			mockFetch.mockResolvedValueOnce(
				createMockResponse(
					{ error: "Not Found" },
					{ ok: false, status: 404, statusText: "Not Found" },
				),
			);
			expect(
				client.createTask({
					name: "task-123",
					owner: "testuser",
					templateName: "nonexistent",
					templatePreset: "Default",
					prompt: "Test prompt",
					organization: "coder",
				}),
			).rejects.toThrow(CoderAPIError);
		});
	});

	describe("sendTaskInput", () => {
		test("sends input successfully", async () => {
			mockFetch.mockResolvedValue(createMockResponse({}));

			await client.sendTaskInput("testuser", "task-123", "Test input");

			expect(mockFetch).toHaveBeenCalledWith(
				"https://coder.test/api/v2/users/testuser/tasks/task-123/send",
				expect.objectContaining({
					method: "POST",
					body: expect.stringContaining("Test input"),
				}),
			);
		});

		test("request body contains input field", async () => {
			mockFetch.mockResolvedValue(createMockResponse({}));

			await client.sendTaskInput("testuser", "task-123", "Test input");

			const call = mockFetch.mock.calls[0];
			const body = JSON.parse(call[1].body);
			expect(body.input).toBe("Test input");
		});

		test("throws error when task not found (404)", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Not Found" },
					{ ok: false, status: 404, statusText: "Not Found" },
				),
			);

			expect(
				client.sendTaskInput("testuser", "task-123", "Test input"),
			).rejects.toThrow(CoderAPIError);
		});

		test("throws error when task not running (400)", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Bad Request" },
					{ ok: false, status: 400, statusText: "Bad Request" },
				),
			);

			expect(
				client.sendTaskInput("testuser", "task-123", "Test input"),
			).rejects.toThrow(CoderAPIError);
		});
	});
});
