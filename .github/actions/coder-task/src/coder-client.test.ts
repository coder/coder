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
	createMockInputs,
	createMockResponse,
} from "./test-helpers";

describe("CoderClient", () => {
	let client: CoderClient;
	let mockFetch: ReturnType<typeof mock>;

	beforeEach(() => {
		const mockInputs = createMockInputs();
		client = new CoderClient(mockInputs.coderUrl, mockInputs.coderToken);
		mockFetch = mock(() => Promise.resolve(createMockResponse([])));
		global.fetch = mockFetch as unknown as typeof fetch;
	});

	describe("getCoderUserByGitHubId", () => {
		test("returns the user when found", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockUserList));
			const result = await client.getCoderUserByGitHubId(
				mockUser.github_com_user_id,
			);
			expect(mockFetch).toHaveBeenCalledWith(
				`https://coder.test/api/v2/users?q=github_com_user_id%3A${mockUser.github_com_user_id!.toString()}`,
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
			mockFetch.mockResolvedValue(createMockResponse(mockUserListDuplicate));
			expect(
				client.getCoderUserByGitHubId(mockUser.github_com_user_id!),
			).rejects.toThrow(CoderAPIError);
			expect(mockFetch).toHaveBeenCalledWith(
				`https://coder.test/api/v2/users?q=github_com_user_id%3A${mockUser.github_com_user_id!.toString()}`,
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
		});

		test("throws an error if no Coder user is found with the given GitHub ID", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockUserListEmpty));
			expect(
				client.getCoderUserByGitHubId(mockUser.github_com_user_id!),
			).rejects.toThrow(CoderAPIError);
			expect(mockFetch).toHaveBeenCalledWith(
				`https://coder.test/api/v2/users?q=github_com_user_id%3A${mockUser.github_com_user_id}`,
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
			expect(
				client.getCoderUserByGitHubId(mockUser.github_com_user_id!),
			).rejects.toThrow(CoderAPIError);
		});

		test("throws error on 500 server error", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Internal Server Error" },
					{ ok: false, status: 500, statusText: "Internal Server Error" },
				),
			);
			expect(
				client.getCoderUserByGitHubId(mockUser.github_com_user_id!),
			).rejects.toThrow(CoderAPIError);
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
			const mockInputs = createMockInputs();
			const result = await client.getTemplateByOrganizationAndName(
				mockInputs.organization,
				mockTemplate.name,
			);
			expect(mockFetch).toHaveBeenCalledWith(
				`https://coder.test/api/v2/organizations/${mockInputs.organization}/templates/${mockTemplate.name}`,
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
			const mockInputs = createMockInputs();
			expect(
				client.getTemplateByOrganizationAndName(
					mockInputs.organization,
					"nonexistent",
				),
			).rejects.toThrow(CoderAPIError);
		});
	});

	describe("getTask", () => {
		test("returns task when task exists", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockTaskList));
			const result = await client.getTask(mockUser.username, mockTask.name);
			expect(result).not.toBeNull();
			expect(result?.id).toBe(mockTask.id);
			expect(result?.name).toBe(mockTask.name);
			expect(mockFetch).toHaveBeenCalledWith(
				`https://coder.test/api/experimental/tasks?q=owner%3A${mockUser.username}`,
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
		});

		test("returns null when task doesn't exist (404)", async () => {
			mockFetch.mockResolvedValue(createMockResponse(mockTaskListEmpty));
			const result = await client.getTask(mockUser.username, mockTask.name);
			expect(result).toBeNull();
			expect(mockFetch).toHaveBeenCalledWith(
				`https://coder.test/api/experimental/tasks?q=owner%3A${mockUser.username}`,
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
			const mockInputs = createMockInputs();
			const result = await client.createTask({
				name: mockTask.name,
				templateName: mockTemplate.name,
				templatePreset: mockInputs.templatePreset,
				organization: mockInputs.organization,
				owner: mockUser.username,
				prompt: mockInputs.taskPrompt,
			});
			expect(result.id).toBe(mockTask.id);
			expect(result.name).toBe(mockTask.name);
			expect(mockFetch).toHaveBeenNthCalledWith(
				1,
				`https://coder.test/api/v2/organizations/${mockInputs.organization}/templates/${mockTemplate.name}`,
				expect.objectContaining({
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
				}),
			);
			expect(mockFetch).toHaveBeenNthCalledWith(
				2,
				`https://coder.test/api/experimental/tasks/${mockUser.username}`,
				expect.objectContaining({
					method: "POST",
					headers: expect.objectContaining({
						"Coder-Session-Token": "test-token",
					}),
					body: JSON.stringify({
						name: mockTask.name,
						template_id: mockTemplate.id,
						template_version_preset_id: mockInputs.templatePreset,
						prompt: mockInputs.taskPrompt,
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
			const mockInputs = createMockInputs();
			expect(
				client.createTask({
					name: mockTask.name,
					owner: mockUser.username,
					templateName: "nonexistent",
					templatePreset: mockInputs.templatePreset,
					prompt: mockInputs.taskPrompt,
					organization: mockInputs.organization,
				}),
			).rejects.toThrow(CoderAPIError);
		});
	});

	describe("sendTaskInput", () => {
		test("sends input successfully", async () => {
			mockFetch.mockResolvedValue(createMockResponse({}));

			const testInput = "Test input";
			await client.sendTaskInput(mockUser.username, mockTask.name, testInput);

			expect(mockFetch).toHaveBeenCalledWith(
				`https://coder.test/api/v2/users/${mockUser.username}/tasks/${mockTask.name}/send`,
				expect.objectContaining({
					method: "POST",
					body: expect.stringContaining(testInput),
				}),
			);
		});

		test("request body contains input field", async () => {
			mockFetch.mockResolvedValue(createMockResponse({}));

			const testInput = "Test input";
			await client.sendTaskInput(mockUser.username, mockTask.name, testInput);

			const call = mockFetch.mock.calls[0];
			const body = JSON.parse(call[1].body);
			expect(body.input).toBe(testInput);
		});

		test("throws error when task not found (404)", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Not Found" },
					{ ok: false, status: 404, statusText: "Not Found" },
				),
			);

			const testInput = "Test input";
			expect(
				client.sendTaskInput(mockUser.username, mockTask.name, testInput),
			).rejects.toThrow(CoderAPIError);
		});

		test("throws error when task not running (400)", async () => {
			mockFetch.mockResolvedValue(
				createMockResponse(
					{ error: "Bad Request" },
					{ ok: false, status: 400, statusText: "Bad Request" },
				),
			);

			const testInput = "Test input";
			expect(
				client.sendTaskInput(mockUser.username, mockTask.name, testInput),
			).rejects.toThrow(CoderAPIError);
		});
	});
});
