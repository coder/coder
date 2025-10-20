import { describe, expect, test, beforeEach, mock } from "bun:test";
import { CoderClient, CoderAPIError } from "./coder-client";
import {
	mockUser,
	mockTemplate,
	mockTaskStatus,
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
		describe("Success Cases", () => {
			test("returns user when found", async () => {
				mockFetch.mockResolvedValue(createMockResponse([mockUser]));

				const result = await client.getCoderUserByGitHubId(12345);

				expect(result.id).toBe(mockUser.id);
				expect(result.username).toBe(mockUser.username);
				expect(result.github_com_user_id).toBe(mockUser.github_com_user_id);
			});

			test("sends correct request", async () => {
				mockFetch.mockResolvedValue(createMockResponse([mockUser]));

				await client.getCoderUserByGitHubId(12345);

				expect(mockFetch).toHaveBeenCalledWith(
					"https://coder.test/api/v2/users?q=github_com_user_id:12345",
					expect.objectContaining({
						headers: expect.objectContaining({
							"Coder-Session-Token": "test-token",
						}),
					}),
				);
			});

			test("uses first user when multiple returned", async () => {
				const secondUser = { ...mockUser, id: "different-id" };
				mockFetch.mockResolvedValue(createMockResponse([mockUser, secondUser]));

				const result = await client.getCoderUserByGitHubId(12345);

				expect(result.id).toBe(mockUser.id);
			});
		});

		describe("Error Cases", () => {
			test("throws error when no users found", async () => {
				mockFetch.mockResolvedValue(createMockResponse([]));

				await expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
					CoderAPIError,
				);
				await expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
					"No Coder user found with GitHub user ID 12345",
				);
			});

			test("throws error on 401 unauthorized", async () => {
				mockFetch.mockResolvedValue(
					createMockResponse(
						{ error: "Unauthorized" },
						{ ok: false, status: 401, statusText: "Unauthorized" },
					),
				);

				await expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
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

				await expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
					CoderAPIError,
				);
			});

			test("throws error on network timeout", async () => {
				mockFetch.mockRejectedValue(new Error("Network timeout"));

				await expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow(
					"Network timeout",
				);
			});

			test("throws error on invalid JSON response", async () => {
				mockFetch.mockResolvedValue({
					ok: true,
					status: 200,
					statusText: "OK",
					json: async () => {
						throw new Error("Invalid JSON");
					},
				});

				await expect(client.getCoderUserByGitHubId(12345)).rejects.toThrow();
			});
		});

		describe("Edge Cases", () => {
			test("handles GitHub user ID of 0", async () => {
				mockFetch.mockResolvedValue(createMockResponse([mockUser]));

				await client.getCoderUserByGitHubId(0);

				expect(mockFetch).toHaveBeenCalledWith(
					"https://coder.test/api/v2/users?q=github_com_user_id:0",
					expect.any(Object),
				);
			});

			test("handles very large GitHub user ID", async () => {
				const largeId = Number.MAX_SAFE_INTEGER;
				mockFetch.mockResolvedValue(createMockResponse([mockUser]));

				await client.getCoderUserByGitHubId(largeId);

				expect(mockFetch).toHaveBeenCalledWith(
					`https://coder.test/api/v2/users?q=github_com_user_id:${largeId}`,
					expect.any(Object),
				);
			});
		});
	});

	describe("getUserByUsername", () => {
		describe("Success Cases", () => {
			test("returns user when found", async () => {
				mockFetch.mockResolvedValue(createMockResponse(mockUser));

				const result = await client.getUserByUsername("testuser");

				expect(result.id).toBe(mockUser.id);
				expect(result.username).toBe(mockUser.username);
			});

			test("uses correct endpoint", async () => {
				mockFetch.mockResolvedValue(createMockResponse(mockUser));

				await client.getUserByUsername("testuser");

				expect(mockFetch).toHaveBeenCalledWith(
					"https://coder.test/api/v2/users/testuser",
					expect.any(Object),
				);
			});

			test("handles special characters in username", async () => {
				mockFetch.mockResolvedValue(createMockResponse(mockUser));

				await client.getUserByUsername("test-user.name");

				expect(mockFetch).toHaveBeenCalledWith(
					"https://coder.test/api/v2/users/test-user.name",
					expect.any(Object),
				);
			});
		});

		describe("Error Cases", () => {
			test("throws error when user not found", async () => {
				mockFetch.mockResolvedValue(
					createMockResponse(
						{ error: "Not Found" },
						{ ok: false, status: 404, statusText: "Not Found" },
					),
				);

				await expect(client.getUserByUsername("nonexistent")).rejects.toThrow(
					CoderAPIError,
				);
			});
		});
	});

	describe("getTemplateByName", () => {
		describe("Success Cases", () => {
			test("returns template when found", async () => {
				mockFetch.mockResolvedValue(createMockResponse([mockTemplate]));

				const result = await client.getTemplateByName("traiage");

				expect(result.id).toBe(mockTemplate.id);
				expect(result.name).toBe(mockTemplate.name);
			});

			test("uses correct endpoint with URL encoding", async () => {
				mockFetch.mockResolvedValue(createMockResponse([mockTemplate]));

				await client.getTemplateByName("traiage");

				expect(mockFetch).toHaveBeenCalledWith(
					"https://coder.test/api/v2/templates?q=exact_name:traiage",
					expect.any(Object),
				);
			});

			test("URL encodes template name with special characters", async () => {
				mockFetch.mockResolvedValue(createMockResponse([mockTemplate]));

				await client.getTemplateByName("template name");

				expect(mockFetch).toHaveBeenCalledWith(
					expect.stringContaining("exact_name:template%20name"),
					expect.any(Object),
				);
			});

			test("uses first template when multiple returned", async () => {
				const secondTemplate = { ...mockTemplate, id: "different-id" };
				mockFetch.mockResolvedValue(
					createMockResponse([mockTemplate, secondTemplate]),
				);

				const result = await client.getTemplateByName("traiage");

				expect(result.id).toBe(mockTemplate.id);
			});
		});

		describe("Error Cases", () => {
			test("throws error when template not found", async () => {
				mockFetch.mockResolvedValue(createMockResponse([]));

				await expect(client.getTemplateByName("nonexistent")).rejects.toThrow(
					CoderAPIError,
				);
				await expect(client.getTemplateByName("nonexistent")).rejects.toThrow(
					'Template "nonexistent" not found',
				);
			});
		});
	});

	describe("getTaskStatus", () => {
		describe("Success Cases", () => {
			test("returns task status when task exists", async () => {
				mockFetch.mockResolvedValue(createMockResponse(mockTaskStatus));

				const result = await client.getTaskStatus("testuser", "task-123");

				expect(result).not.toBeNull();
				expect(result?.id).toBe(mockTaskStatus.id);
				expect(result?.name).toBe(mockTaskStatus.name);
			});

			test("uses correct endpoint", async () => {
				mockFetch.mockResolvedValue(createMockResponse(mockTaskStatus));

				await client.getTaskStatus("testuser", "task-123");

				expect(mockFetch).toHaveBeenCalledWith(
					"https://coder.test/api/v2/users/testuser/tasks/task-123",
					expect.any(Object),
				);
			});

			test("returns null when task doesn't exist (404)", async () => {
				mockFetch.mockResolvedValue(
					createMockResponse(
						{ error: "Not Found" },
						{ ok: false, status: 404, statusText: "Not Found" },
					),
				);

				const result = await client.getTaskStatus("testuser", "task-123");

				expect(result).toBeNull();
			});
		});

		describe("Error Cases", () => {
			test("throws error on API error other than 404", async () => {
				mockFetch.mockResolvedValue(
					createMockResponse(
						{ error: "Internal Server Error" },
						{ ok: false, status: 500, statusText: "Internal Server Error" },
					),
				);

				await expect(
					client.getTaskStatus("testuser", "task-123"),
				).rejects.toThrow(CoderAPIError);
			});
		});

		describe("Edge Cases", () => {
			test("handles task name with special characters", async () => {
				mockFetch.mockResolvedValue(createMockResponse(mockTaskStatus));

				await client.getTaskStatus("testuser", "task-gh-123");

				expect(mockFetch).toHaveBeenCalledWith(
					"https://coder.test/api/v2/users/testuser/tasks/task-gh-123",
					expect.any(Object),
				);
			});

			test("handles very long task names", async () => {
				const longName = "task-" + "x".repeat(200);
				mockFetch.mockResolvedValue(createMockResponse(mockTaskStatus));

				await client.getTaskStatus("testuser", longName);

				expect(mockFetch).toHaveBeenCalledWith(
					`https://coder.test/api/v2/users/testuser/tasks/${longName}`,
					expect.any(Object),
				);
			});
		});
	});

	describe("createTask", () => {
		describe("Success Cases", () => {
			test("creates task successfully", async () => {
				// Mock getTemplateByName
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				// Mock getUserByUsername
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				// Mock createTask POST
				mockFetch.mockResolvedValueOnce(createMockResponse(mockTaskStatus));

				const result = await client.createTask({
					name: "task-123",
					owner: "testuser",
					templateName: "traiage",
					templatePreset: "Default",
					prompt: "Test prompt",
					organization: "coder",
				});

				expect(result.id).toBe(mockTaskStatus.id);
				expect(result.name).toBe(mockTaskStatus.name);
			});

			test("calls getTemplateByName to resolve template ID", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockTaskStatus));

				await client.createTask({
					name: "task-123",
					owner: "testuser",
					templateName: "traiage",
					templatePreset: "Default",
					prompt: "Test prompt",
					organization: "coder",
				});

				// First call should be getTemplateByName
				expect(mockFetch).toHaveBeenNthCalledWith(
					1,
					expect.stringContaining("/api/v2/templates"),
					expect.any(Object),
				);
			});

			test("calls getUserByUsername to resolve user ID", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockTaskStatus));

				await client.createTask({
					name: "task-123",
					owner: "testuser",
					templateName: "traiage",
					templatePreset: "Default",
					prompt: "Test prompt",
					organization: "coder",
				});

				// Second call should be getUserByUsername
				expect(mockFetch).toHaveBeenNthCalledWith(
					2,
					"https://coder.test/api/v2/users/testuser",
					expect.any(Object),
				);
			});

			test("sends correct POST request body", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockTaskStatus));

				await client.createTask({
					name: "task-123",
					owner: "testuser",
					templateName: "traiage",
					templatePreset: "Default",
					prompt: "Test prompt",
					organization: "coder",
				});

				// Third call should be POST to create task
				expect(mockFetch).toHaveBeenNthCalledWith(
					3,
					"https://coder.test/api/v2/organizations/coder/members/testuser/tasks",
					expect.objectContaining({
						method: "POST",
						body: expect.stringContaining("task-123"),
					}),
				);
			});
		});

		describe("Error Cases", () => {
			test("throws error when template not found", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([]));

				await expect(
					client.createTask({
						name: "task-123",
						owner: "testuser",
						templateName: "nonexistent",
						templatePreset: "Default",
						prompt: "Test prompt",
						organization: "coder",
					}),
				).rejects.toThrow('Template "nonexistent" not found');
			});

			test("throws error when user not found", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(
					createMockResponse(
						{ error: "Not Found" },
						{ ok: false, status: 404, statusText: "Not Found" },
					),
				);

				await expect(
					client.createTask({
						name: "task-123",
						owner: "nonexistent",
						templateName: "traiage",
						templatePreset: "Default",
						prompt: "Test prompt",
						organization: "coder",
					}),
				).rejects.toThrow(CoderAPIError);
			});

			test("throws error on duplicate task name (409 conflict)", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				mockFetch.mockResolvedValueOnce(
					createMockResponse(
						{ error: "Conflict" },
						{ ok: false, status: 409, statusText: "Conflict" },
					),
				);

				await expect(
					client.createTask({
						name: "task-123",
						owner: "testuser",
						templateName: "traiage",
						templatePreset: "Default",
						prompt: "Test prompt",
						organization: "coder",
					}),
				).rejects.toThrow(CoderAPIError);
			});

			test("throws error on permission denied (403)", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				mockFetch.mockResolvedValueOnce(
					createMockResponse(
						{ error: "Forbidden" },
						{ ok: false, status: 403, statusText: "Forbidden" },
					),
				);

				await expect(
					client.createTask({
						name: "task-123",
						owner: "testuser",
						templateName: "traiage",
						templatePreset: "Default",
						prompt: "Test prompt",
						organization: "coder",
					}),
				).rejects.toThrow(CoderAPIError);
			});
		});

		describe("Edge Cases", () => {
			test("handles very long prompts", async () => {
				const longPrompt = "x".repeat(50000);
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockTaskStatus));

				await client.createTask({
					name: "task-123",
					owner: "testuser",
					templateName: "traiage",
					templatePreset: "Default",
					prompt: longPrompt,
					organization: "coder",
				});

				expect(mockFetch).toHaveBeenCalledTimes(3);
			});

			test("handles special characters in task name", async () => {
				mockFetch.mockResolvedValueOnce(createMockResponse([mockTemplate]));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockUser));
				mockFetch.mockResolvedValueOnce(createMockResponse(mockTaskStatus));

				await client.createTask({
					name: "task-gh-123",
					owner: "testuser",
					templateName: "traiage",
					templatePreset: "Default",
					prompt: "Test prompt",
					organization: "coder",
				});

				expect(mockFetch).toHaveBeenNthCalledWith(
					3,
					expect.any(String),
					expect.objectContaining({
						body: expect.stringContaining("task-gh-123"),
					}),
				);
			});
		});
	});

	describe("sendTaskInput", () => {
		describe("Success Cases", () => {
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
		});

		describe("Error Cases", () => {
			test("throws error when task not found (404)", async () => {
				mockFetch.mockResolvedValue(
					createMockResponse(
						{ error: "Not Found" },
						{ ok: false, status: 404, statusText: "Not Found" },
					),
				);

				await expect(
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

				await expect(
					client.sendTaskInput("testuser", "task-123", "Test input"),
				).rejects.toThrow(CoderAPIError);
			});
		});
	});

	describe("CoderAPIError", () => {
		test("contains statusCode", () => {
			const error = new CoderAPIError("Test error", 404);

			expect(error.statusCode).toBe(404);
		});

		test("contains message", () => {
			const error = new CoderAPIError("Test error", 404);

			expect(error.message).toBe("Test error");
		});

		test("contains optional response body", () => {
			const error = new CoderAPIError("Test error", 404, "Response body");

			expect(error.response).toBe("Response body");
		});

		test("is instance of Error", () => {
			const error = new CoderAPIError("Test error", 404);

			expect(error).toBeInstanceOf(Error);
		});

		test("is instance of CoderAPIError", () => {
			const error = new CoderAPIError("Test error", 404);

			expect(error).toBeInstanceOf(CoderAPIError);
		});
	});
});
