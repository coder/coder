import {
	MockProvisionerJob,
	MockStoppedWorkspace,
	MockTemplate,
	MockTemplateVersion2,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockWorkspace,
	MockWorkspaceBuild,
	MockWorkspaceBuildParameter1,
} from "#/testHelpers/entities";
import { API, getURLWithSearchParams, MissingBuildParameters } from "./api";
import type * as TypesGen from "./typesGenerated";

const axiosInstance = API.getAxiosInstance();

describe("api.ts", () => {
	describe("login", () => {
		it("should return LoginResponse", async () => {
			// given
			const loginResponse: TypesGen.LoginWithPasswordResponse = {
				session_token: "abc_123_test",
			};

			vi.spyOn(axiosInstance, "post").mockResolvedValueOnce({
				data: loginResponse,
			});

			// when
			const result = await API.login("test", "123");

			// then
			expect(axiosInstance.post).toHaveBeenCalled();
			expect(result).toStrictEqual(loginResponse);
		});

		it("should throw an error on 401", async () => {
			// given
			// ..ensure that we await our expect assertion in async/await test
			expect.assertions(1);
			const expectedError = {
				message: "Validation failed",
				errors: [{ field: "email", code: "email" }],
			};
			const axiosMockPost = vi.fn().mockImplementationOnce(() => {
				return Promise.reject(expectedError);
			});
			axiosInstance.post = axiosMockPost;

			try {
				await API.login("test", "123");
			} catch (error) {
				expect(error).toStrictEqual(expectedError);
			}
		});
	});

	describe("logout", () => {
		it("should return without erroring", async () => {
			// given
			const axiosMockPost = vi.fn().mockImplementationOnce(() => {
				return Promise.resolve();
			});
			axiosInstance.post = axiosMockPost;

			// when
			await API.logout();

			// then
			expect(axiosMockPost).toHaveBeenCalled();
		});

		it("should throw an error on 500", async () => {
			// given
			// ..ensure that we await our expect assertion in async/await test
			expect.assertions(1);
			const expectedError = {
				message: "Failed to logout.",
			};
			const axiosMockPost = vi.fn().mockImplementationOnce(() => {
				return Promise.reject(expectedError);
			});

			axiosInstance.post = axiosMockPost;

			try {
				await API.logout();
			} catch (error) {
				expect(error).toStrictEqual(expectedError);
			}
		});
	});

	describe("getApiKey", () => {
		it("should return APIKeyResponse", async () => {
			// given
			const apiKeyResponse: TypesGen.GenerateAPIKeyResponse = {
				key: "abc_123_test",
			};
			const axiosMockPost = vi.fn().mockImplementationOnce(() => {
				return Promise.resolve({ data: apiKeyResponse });
			});

			axiosInstance.post = axiosMockPost;

			// when
			const result = await API.getApiKey();

			// then
			expect(axiosMockPost).toHaveBeenCalled();
			expect(result).toStrictEqual(apiKeyResponse);
		});

		it("should throw an error on 401", async () => {
			// given
			// ..ensure that we await our expect assertion in async/await test
			expect.assertions(1);
			const expectedError = {
				message: "No Cookie!",
			};
			const axiosMockPost = vi.fn().mockImplementationOnce(() => {
				return Promise.reject(expectedError);
			});

			axiosInstance.post = axiosMockPost;

			try {
				await API.getApiKey();
			} catch (error) {
				expect(error).toStrictEqual(expectedError);
			}
		});
	});

	describe("getURLWithSearchParams - workspaces", () => {
		it.each<[string, TypesGen.WorkspaceFilter | undefined, string]>([
			["/api/v2/workspaces", undefined, "/api/v2/workspaces"],

			["/api/v2/workspaces", { q: "" }, "/api/v2/workspaces"],
			[
				"/api/v2/workspaces",
				{ q: "owner:1" },
				"/api/v2/workspaces?q=owner%3A1",
			],

			[
				"/api/v2/workspaces",
				{ q: "owner:me" },
				"/api/v2/workspaces?q=owner%3Ame",
			],
		])("Workspaces - getURLWithSearchParams(%p, %p) returns %p", (basePath, filter, expected) => {
			expect(getURLWithSearchParams(basePath, filter)).toBe(expected);
		});
	});

	describe("getURLWithSearchParams - users", () => {
		it.each<[string, TypesGen.UsersRequest | undefined, string]>([
			["/api/v2/users", undefined, "/api/v2/users"],
			[
				"/api/v2/users",
				{ q: "status:active" },
				"/api/v2/users?q=status%3Aactive",
			],
			["/api/v2/users", { q: "" }, "/api/v2/users"],
		])("Users - getURLWithSearchParams(%p, %p) returns %p", (basePath, filter, expected) => {
			expect(getURLWithSearchParams(basePath, filter)).toBe(expected);
		});
	});

	describe("AI spend requests", () => {
		const window = {
			period_start: "2026-07-01T00:00:00Z",
			period_end: "2026-08-01T00:00:00Z",
		};

		// Each endpoint's request, URL path, and response for the given IDs.
		const endpoints = [
			{
				name: "getOrganizationGroupsAISpend",
				path: "/api/v2/organizations/my-org/groups/ai/spend",
				request: (ids: string[]) =>
					API.getOrganizationGroupsAISpend("my-org", ids),
				response: (ids: string[]) => ({
					...window,
					groups: ids.map((id) => ({
						group_id: id,
						spend_micros: 0,
						budget: null,
					})),
				}),
			},
			{
				name: "getGroupMembersAISpend",
				path: "/api/v2/groups/group-1/members/ai/spend",
				request: (ids: string[]) => API.getGroupMembersAISpend("group-1", ids),
				response: (ids: string[]) => ({
					...window,
					members: ids.map((id) => ({
						user_id: id,
						effective_group_id: null,
						group_budget: null,
						group_spend_micros: 0,
					})),
				}),
			},
		];

		afterEach(() => {
			// The suite doesn't auto-restore mocks; don't leak the stubs.
			vi.restoreAllMocks();
		});

		describe.each(endpoints)("$name", ({ path, request, response }) => {
			it("rejects an empty ID list without sending a request", async () => {
				const getSpy = vi
					.spyOn(axiosInstance, "get")
					.mockResolvedValue({ data: {} });

				await expect(request([])).rejects.toThrow(/must not be empty/);
				expect(getSpy).not.toHaveBeenCalled();
			});

			it("sends a single request for up to 100 IDs", async () => {
				const ids = Array.from({ length: 25 }, (_, i) => `id-${i}`);
				const getSpy = vi
					.spyOn(axiosInstance, "get")
					.mockResolvedValueOnce({ data: response(ids) });

				const result = await request(ids);

				expect(getSpy).toHaveBeenCalledTimes(1);
				expect(getSpy.mock.calls[0][0]).toContain(path);
				expect(getSpy.mock.calls[0][0]).toContain(
					encodeURIComponent(ids.join(",")),
				);
				expect(result).toStrictEqual(response(ids));
			});

			it("batches requests of 100 IDs and merges the results", async () => {
				const ids = Array.from({ length: 150 }, (_, i) => `id-${i}`);
				const getSpy = vi
					.spyOn(axiosInstance, "get")
					.mockResolvedValueOnce({ data: response(ids.slice(0, 100)) })
					.mockResolvedValueOnce({ data: response(ids.slice(100)) });

				const result = await request(ids);

				expect(getSpy).toHaveBeenCalledTimes(2);
				expect(getSpy.mock.calls[0][0]).toContain(
					encodeURIComponent(ids.slice(0, 100).join(",")),
				);
				expect(getSpy.mock.calls[1][0]).toContain(
					encodeURIComponent(ids.slice(100).join(",")),
				);
				expect(result).toStrictEqual(response(ids));
			});
		});
	});

	describe("update", () => {
		describe("given a running workspace", () => {
			it("stops with current version before starting with the latest version", async () => {
				vi.spyOn(API, "postWorkspaceBuild").mockResolvedValueOnce({
					...MockWorkspaceBuild,
					transition: "stop",
				});
				vi.spyOn(API, "postWorkspaceBuild").mockResolvedValueOnce({
					...MockWorkspaceBuild,
					template_version_id: MockTemplateVersion2.id,
					transition: "start",
				});
				vi.spyOn(API, "getTemplate").mockResolvedValueOnce({
					...MockTemplate,
					active_version_id: MockTemplateVersion2.id,
				});
				await API.updateWorkspace(MockWorkspace);
				expect(API.postWorkspaceBuild).toHaveBeenCalledWith(MockWorkspace.id, {
					transition: "stop",
					log_level: undefined,
				});
				expect(API.postWorkspaceBuild).toHaveBeenCalledWith(MockWorkspace.id, {
					transition: "start",
					template_version_id: MockTemplateVersion2.id,
					rich_parameter_values: [],
				});
			});

			it("fails when having missing parameters", async () => {
				vi.spyOn(API, "postWorkspaceBuild").mockResolvedValue(
					MockWorkspaceBuild,
				);
				vi.spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
				vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValue([]);
				vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValue([
					MockTemplateVersionParameter1,
					{ ...MockTemplateVersionParameter2, mutable: false },
				]);

				let error = new Error();
				try {
					await API.updateWorkspace(MockWorkspace);
				} catch (e) {
					error = e as Error;
				}

				expect(error).toBeInstanceOf(MissingBuildParameters);
				// Verify if the correct missing parameters are being passed
				expect((error as MissingBuildParameters).parameters).toEqual([
					MockTemplateVersionParameter1,
					{ ...MockTemplateVersionParameter2, mutable: false },
				]);
			});

			it("creates a build with no parameters if it is already filled", async () => {
				vi.spyOn(API, "postWorkspaceBuild").mockResolvedValueOnce({
					...MockWorkspaceBuild,
					transition: "stop",
				});
				vi.spyOn(API, "postWorkspaceBuild").mockResolvedValueOnce({
					...MockWorkspaceBuild,
					template_version_id: MockTemplateVersion2.id,
					transition: "start",
				});
				vi.spyOn(API, "getTemplate").mockResolvedValueOnce(MockTemplate);
				vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValue([
					MockWorkspaceBuildParameter1,
				]);
				vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValue([
					{
						...MockTemplateVersionParameter1,
						required: true,
						mutable: false,
					},
				]);
				await API.updateWorkspace(MockWorkspace);
				expect(API.postWorkspaceBuild).toHaveBeenCalledWith(MockWorkspace.id, {
					transition: "stop",
					log_level: undefined,
				});
				expect(API.postWorkspaceBuild).toHaveBeenCalledWith(MockWorkspace.id, {
					transition: "start",
					template_version_id: MockTemplate.active_version_id,
					rich_parameter_values: [],
				});
			});
		});
		describe("given a stopped workspace", () => {
			it("creates a build with start and the latest template", async () => {
				vi.spyOn(API, "postWorkspaceBuild").mockResolvedValueOnce(
					MockWorkspaceBuild,
				);
				vi.spyOn(API, "getTemplate").mockResolvedValueOnce({
					...MockTemplate,
					active_version_id: MockTemplateVersion2.id,
				});
				await API.updateWorkspace(MockStoppedWorkspace);
				expect(API.postWorkspaceBuild).toHaveBeenCalledWith(
					MockStoppedWorkspace.id,
					{
						transition: "start",
						template_version_id: MockTemplateVersion2.id,
						rich_parameter_values: [],
					},
				);
			});
		});
	});

	describe("changeWorkspaceVersion", () => {
		it("stops workspace before changing version if running", async () => {
			vi.spyOn(API, "stopWorkspace").mockResolvedValueOnce({
				...MockWorkspaceBuild,
				transition: "stop",
			});
			vi.spyOn(API, "waitForBuild").mockResolvedValueOnce({
				...MockProvisionerJob,
				status: "succeeded",
			});
			vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([]);
			vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce(
				[],
			);
			vi.spyOn(API, "postWorkspaceBuild").mockResolvedValueOnce({
				...MockWorkspaceBuild,
				template_version_id: MockTemplateVersion2.id,
				transition: "start",
			});

			await API.changeWorkspaceVersion(MockWorkspace, MockTemplateVersion2.id);

			expect(API.stopWorkspace).toHaveBeenCalledWith(MockWorkspace.id);
			expect(API.postWorkspaceBuild).toHaveBeenCalledWith(MockWorkspace.id, {
				transition: "start",
				template_version_id: MockTemplateVersion2.id,
				rich_parameter_values: [],
			});
		});

		it("does not stop workspace if already stopped", async () => {
			vi.spyOn(API, "stopWorkspace");
			vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([]);
			vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce(
				[],
			);
			vi.spyOn(API, "postWorkspaceBuild").mockResolvedValueOnce({
				...MockWorkspaceBuild,
				template_version_id: MockTemplateVersion2.id,
				transition: "start",
			});

			await API.changeWorkspaceVersion(
				MockStoppedWorkspace,
				MockTemplateVersion2.id,
			);

			expect(API.stopWorkspace).not.toHaveBeenCalled();
		});

		it("rejects if stop is canceled", async () => {
			vi.spyOn(API, "stopWorkspace").mockResolvedValueOnce({
				...MockWorkspaceBuild,
				transition: "stop",
			});
			vi.spyOn(API, "waitForBuild").mockResolvedValueOnce({
				...MockProvisionerJob,
				status: "canceled",
			});
			vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([]);
			vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce(
				[],
			);
			vi.spyOn(API, "postWorkspaceBuild");

			await expect(
				API.changeWorkspaceVersion(MockWorkspace, MockTemplateVersion2.id),
			).rejects.toThrow("Workspace stop was canceled");
			expect(API.postWorkspaceBuild).not.toHaveBeenCalled();
		});

		it("throws MissingBuildParameters for missing params", async () => {
			vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([]);
			vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce([
				MockTemplateVersionParameter1,
				{ ...MockTemplateVersionParameter2, mutable: false },
			]);

			let error = new Error();
			try {
				await API.changeWorkspaceVersion(
					MockStoppedWorkspace,
					MockTemplateVersion2.id,
				);
			} catch (e) {
				error = e as Error;
			}

			expect(error).toBeInstanceOf(MissingBuildParameters);
			expect((error as MissingBuildParameters).parameters).toEqual([
				MockTemplateVersionParameter1,
				{ ...MockTemplateVersionParameter2, mutable: false },
			]);
		});
	});

	describe("chat configuration endpoints", () => {
		it.each<[string, () => Promise<unknown>, unknown]>([
			[
				"/api/experimental/chats/models",
				() => API.experimental.getChatModels(),
				{
					providers: [],
				},
			],
			[
				"/api/experimental/chats/model-configs",
				() => API.experimental.getChatModelConfigs(),
				[],
			],
		])("returns response data for %s", async (path, request, responseData) => {
			vi.spyOn(axiosInstance, "get").mockResolvedValueOnce({
				data: responseData,
			});

			const result = await request();

			expect(axiosInstance.get).toHaveBeenCalledWith(path);
			expect(result).toStrictEqual(responseData);
		});

		it.each<[string, () => Promise<unknown>]>([
			[
				"/api/experimental/chats/models",
				() => API.experimental.getChatModels(),
			],
			[
				"/api/experimental/chats/model-configs",
				() => API.experimental.getChatModelConfigs(),
			],
		])("rethrows axios errors for %s", async (path, request) => {
			const expectedError = new Error("request failed");
			vi.spyOn(axiosInstance, "get").mockRejectedValueOnce(expectedError);

			await expect(request()).rejects.toBe(expectedError);
			expect(axiosInstance.get).toHaveBeenCalledWith(path);
		});
	});

	describe("user secrets endpoints", () => {
		const userId = "me";
		const secretName = "EXAMPLE_TOKEN";
		const secretNameWithPathChars = "foo%2Fbar value";
		const userSecret: TypesGen.UserSecret = {
			id: "00000000-0000-0000-0000-000000000001",
			name: secretName,
			description: "Example token for tests",
			env_name: secretName,
			file_path: "",
			created_at: "2026-05-04T00:00:00Z",
			updated_at: "2026-05-04T00:00:00Z",
		};

		it("lists user secrets with the correct method and URL", async () => {
			const axiosMockGet = vi.fn().mockResolvedValueOnce({
				data: [userSecret],
			});
			axiosInstance.get = axiosMockGet;

			const result = await API.getUserSecrets(userId);

			expect(axiosMockGet).toHaveBeenCalledWith("/api/v2/users/me/secrets");
			expect(result).toStrictEqual([userSecret]);
		});

		it("gets a user secret with the correct method and URL", async () => {
			const axiosMockGet = vi.fn().mockResolvedValueOnce({
				data: userSecret,
			});
			axiosInstance.get = axiosMockGet;

			const result = await API.getUserSecret(userId, secretNameWithPathChars);

			expect(axiosMockGet).toHaveBeenCalledWith(
				"/api/v2/users/me/secrets/foo%252Fbar%20value",
			);
			expect(result).toStrictEqual(userSecret);
		});

		it("creates a user secret with the correct method and URL", async () => {
			const request: TypesGen.CreateUserSecretRequest = {
				name: secretName,
				value: "",
				description: "Example token for tests",
				env_name: secretName,
			};
			const axiosMockPost = vi.fn().mockResolvedValueOnce({
				data: userSecret,
			});
			axiosInstance.post = axiosMockPost;

			const result = await API.createUserSecret(userId, request);

			expect(axiosMockPost).toHaveBeenCalledWith(
				"/api/v2/users/me/secrets",
				request,
			);
			expect(result).toStrictEqual(userSecret);
		});

		it("updates a user secret with the correct method and URL", async () => {
			const request: TypesGen.UpdateUserSecretRequest = {
				description: "Updated example token for tests",
			};
			const updatedSecret: TypesGen.UserSecret = {
				...userSecret,
				description: "Updated example token for tests",
				updated_at: "2026-05-04T00:01:00Z",
			};
			const axiosMockPatch = vi.fn().mockResolvedValueOnce({
				data: updatedSecret,
			});
			axiosInstance.patch = axiosMockPatch;

			const result = await API.updateUserSecret(
				userId,
				secretNameWithPathChars,
				request,
			);

			expect(axiosMockPatch).toHaveBeenCalledWith(
				"/api/v2/users/me/secrets/foo%252Fbar%20value",
				request,
			);
			expect(result).toStrictEqual(updatedSecret);
		});

		it("deletes a user secret with the correct method and URL", async () => {
			const axiosMockDelete = vi.fn().mockResolvedValueOnce(undefined);
			axiosInstance.delete = axiosMockDelete;

			await API.deleteUserSecret(userId, secretNameWithPathChars);

			expect(axiosMockDelete).toHaveBeenCalledWith(
				"/api/v2/users/me/secrets/foo%252Fbar%20value",
			);
		});
	});

	describe("chat ACL endpoints", () => {
		const chatId = "chat-1";
		const chatACL: TypesGen.ChatACL = {
			users: [],
			groups: [],
		};

		it("gets a chat ACL", async () => {
			vi.spyOn(axiosInstance, "get").mockResolvedValueOnce({
				data: chatACL,
			});

			const result = await API.experimental.getChatACL(chatId);

			expect(axiosInstance.get).toHaveBeenCalledWith(
				`/api/experimental/chats/${chatId}/acl`,
			);
			expect(result).toStrictEqual(chatACL);
		});

		it("updates a chat ACL", async () => {
			const request: TypesGen.UpdateChatACL = {
				user_roles: { "user-1": "read" },
			};

			vi.spyOn(axiosInstance, "patch").mockResolvedValueOnce({});

			await API.experimental.updateChatACL(chatId, request);

			expect(axiosInstance.patch).toHaveBeenCalledWith(
				`/api/experimental/chats/${chatId}/acl`,
				request,
			);
		});
	});
});
