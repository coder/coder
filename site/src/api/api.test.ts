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

	describe("organization chat resource endpoints", () => {
		const organization = "org/name";
		const modelConfigId = "model/id";
		const serverId = "server/id";
		const organizationPath = "/api/experimental/organizations/org%2Fname";
		const modelConfigPath = "/api/experimental/chat-model-configs/model%2Fid";
		const serverPath = "/api/experimental/mcp-servers/server%2Fid";

		it.each<[string, () => Promise<unknown>, unknown]>([
			[
				`${organizationPath}/chats/models`,
				() => API.experimental.getChatModels(organization),
				{ providers: [] },
			],
			[
				`${organizationPath}/chat-model-configs`,
				() => API.experimental.getChatModelConfigs(organization),
				[],
			],
			[
				modelConfigPath,
				() => API.experimental.getChatModelConfig(modelConfigId),
				{ id: modelConfigId },
			],
			[
				`${modelConfigPath}/acl`,
				() => API.experimental.getChatModelConfigACL(modelConfigId),
				{ users: [], groups: [] },
			],
			[
				`${organizationPath}/mcp-servers`,
				() => API.experimental.getMCPServerConfigs(organization),
				[],
			],
			[
				serverPath,
				() => API.experimental.getMCPServerConfig(serverId),
				{ id: serverId },
			],
			[
				`${serverPath}/acl`,
				() => API.experimental.getMCPServerConfigACL(serverId),
				{ users: [], groups: [] },
			],
			[
				`${organizationPath}/chats/config/model-override/general`,
				() => API.experimental.getChatModelOverride(organization, "general"),
				{},
			],
			[
				`${organizationPath}/chats/config/user-personal-model-overrides`,
				() => API.experimental.getUserChatPersonalModelOverrides(organization),
				{},
			],
			[
				`${organizationPath}/chats/config/advisor`,
				() => API.experimental.getChatAdvisorConfig(organization),
				{},
			],
			[
				`${organizationPath}/chats/config/user-compaction-thresholds`,
				() => API.experimental.getUserChatCompactionThresholds(organization),
				{},
			],
		])("gets %s and returns response data", async (path, request, data) => {
			vi.spyOn(axiosInstance, "get").mockResolvedValueOnce({ data });

			await expect(request()).resolves.toStrictEqual(data);
			expect(axiosInstance.get).toHaveBeenCalledWith(path);
		});

		it("builds the item OAuth connect URL", () => {
			expect(API.experimental.getMCPServerOAuth2ConnectURL(serverId)).toBe(
				`${serverPath}/oauth2/connect`,
			);
		});

		it.each<[string, () => Promise<unknown>]>([
			[
				`${organizationPath}/chats/models`,
				() => API.experimental.getChatModels(organization),
			],
			[
				`${organizationPath}/chat-model-configs`,
				() => API.experimental.getChatModelConfigs(organization),
			],
			[
				modelConfigPath,
				() => API.experimental.getChatModelConfig(modelConfigId),
			],
			[
				`${organizationPath}/mcp-servers`,
				() => API.experimental.getMCPServerConfigs(organization),
			],
			[serverPath, () => API.experimental.getMCPServerConfig(serverId)],
		])("rethrows axios errors for %s", async (path, request) => {
			const expectedError = new Error("request failed");
			vi.spyOn(axiosInstance, "get").mockRejectedValueOnce(expectedError);

			await expect(request()).rejects.toBe(expectedError);
			expect(axiosInstance.get).toHaveBeenCalledWith(path);
		});

		it("creates organization model and MCP configs", async () => {
			const modelRequest = {} as TypesGen.CreateChatModelConfigRequest;
			const serverRequest = {} as TypesGen.CreateMCPServerConfigRequest;
			vi.spyOn(axiosInstance, "post")
				.mockResolvedValueOnce({ data: { id: modelConfigId } })
				.mockResolvedValueOnce({ data: { id: serverId } });

			await API.experimental.createChatModelConfig(organization, modelRequest);
			await API.experimental.createMCPServerConfig(organization, serverRequest);

			expect(axiosInstance.post).toHaveBeenNthCalledWith(
				1,
				`${organizationPath}/chat-model-configs`,
				modelRequest,
			);
			expect(axiosInstance.post).toHaveBeenNthCalledWith(
				2,
				`${organizationPath}/mcp-servers`,
				serverRequest,
			);
		});

		it("updates item, ACL, and organization model-bearing settings routes", async () => {
			const modelRequest = {} as TypesGen.UpdateChatModelConfigRequest;
			const serverRequest = {} as TypesGen.UpdateMCPServerConfigRequest;
			const modelACLRequest = {} as TypesGen.UpdateChatModelConfigACL;
			const serverACLRequest = {} as TypesGen.UpdateMCPServerConfigACL;
			const overrideRequest = {} as TypesGen.UpdateChatModelOverrideRequest;
			const personalRequest =
				{} as TypesGen.UpdateUserChatPersonalModelOverrideRequest;
			const advisorRequest = {} as TypesGen.UpdateAdvisorConfigRequest;
			const thresholdRequest =
				{} as TypesGen.UpdateUserChatCompactionThresholdRequest;
			vi.spyOn(axiosInstance, "patch").mockResolvedValue({ data: {} });

			await API.experimental.updateChatModelConfig(modelConfigId, modelRequest);
			await API.experimental.updateChatModelConfigACL(
				modelConfigId,
				modelACLRequest,
			);
			await API.experimental.updateMCPServerConfig(serverId, serverRequest);
			await API.experimental.updateMCPServerConfigACL(
				serverId,
				serverACLRequest,
			);
			await API.experimental.updateChatModelOverride(
				organization,
				"general",
				overrideRequest,
			);
			await API.experimental.updateUserChatPersonalModelOverride(
				organization,
				"root",
				personalRequest,
			);
			await API.experimental.updateChatAdvisorConfig(
				organization,
				advisorRequest,
			);
			await API.experimental.updateUserChatCompactionThreshold(
				organization,
				modelConfigId,
				thresholdRequest,
			);

			expect(axiosInstance.patch).toHaveBeenCalledWith(
				modelConfigPath,
				modelRequest,
			);
			expect(axiosInstance.patch).toHaveBeenCalledWith(
				`${modelConfigPath}/acl`,
				modelACLRequest,
			);
			expect(axiosInstance.patch).toHaveBeenCalledWith(
				serverPath,
				serverRequest,
			);
			expect(axiosInstance.patch).toHaveBeenCalledWith(
				`${serverPath}/acl`,
				serverACLRequest,
			);
			expect(axiosInstance.patch).toHaveBeenCalledWith(
				`${organizationPath}/chats/config/model-override/general`,
				overrideRequest,
			);
			expect(axiosInstance.patch).toHaveBeenCalledWith(
				`${organizationPath}/chats/config/user-personal-model-overrides/root`,
				personalRequest,
			);
			expect(axiosInstance.patch).toHaveBeenCalledWith(
				`${organizationPath}/chats/config/advisor`,
				advisorRequest,
			);
			expect(axiosInstance.patch).toHaveBeenCalledWith(
				`${organizationPath}/chats/config/user-compaction-thresholds/model%2Fid`,
				thresholdRequest,
			);
		});

		it("deletes item, OAuth, and organization threshold routes", async () => {
			vi.spyOn(axiosInstance, "delete").mockResolvedValue({ data: {} });

			await API.experimental.deleteChatModelConfig(modelConfigId);
			await API.experimental.deleteMCPServerConfig(serverId);
			await API.experimental.disconnectMCPServerOAuth2(serverId);
			await API.experimental.deleteUserChatCompactionThreshold(
				organization,
				modelConfigId,
			);

			expect(axiosInstance.delete).toHaveBeenCalledWith(modelConfigPath);
			expect(axiosInstance.delete).toHaveBeenCalledWith(serverPath);
			expect(axiosInstance.delete).toHaveBeenCalledWith(
				`${serverPath}/oauth2/disconnect`,
			);
			expect(axiosInstance.delete).toHaveBeenCalledWith(
				`${organizationPath}/chats/config/user-compaction-thresholds/model%2Fid`,
			);
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
