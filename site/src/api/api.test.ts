import {
	MockStoppedWorkspace,
	MockTemplate,
	MockTemplateVersion2,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockWorkspace,
	MockWorkspaceBuild,
	MockWorkspaceBuildParameter1,
} from "testHelpers/entities";
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
		])(
			"Workspaces - getURLWithSearchParams(%p, %p) returns %p",
			(basePath, filter, expected) => {
				expect(getURLWithSearchParams(basePath, filter)).toBe(expected);
			},
		);
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
		])(
			"Users - getURLWithSearchParams(%p, %p) returns %p",
			(basePath, filter, expected) => {
				expect(getURLWithSearchParams(basePath, filter)).toBe(expected);
			},
		);
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
});
