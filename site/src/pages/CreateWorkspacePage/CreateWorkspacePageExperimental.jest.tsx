import {
	MockDropdownParameter,
	MockDynamicParametersResponse,
	MockDynamicParametersResponseWithError,
	MockPermissions,
	MockSliderParameter,
	MockTemplate,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
	MockValidationParameter,
	MockWorkspace,
} from "testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { createMockWebSocket } from "testHelpers/websockets";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import type { DynamicParametersResponse } from "api/typesGenerated";
import CreateWorkspacePage from "./CreateWorkspacePage";

describe("CreateWorkspacePage", () => {
	const renderCreateWorkspacePage = (
		route = `/templates/${MockTemplate.name}/workspace`,
	) => {
		return renderWithAuth(<CreateWorkspacePage />, {
			route,
			path: "/templates/:template/workspace",
			extraRoutes: [
				{
					path: "/:username/:workspace",
					element: <div>Workspace Page</div>,
				},
			],
		});
	};

	beforeEach(() => {
		jest.clearAllMocks();

		jest.spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		jest.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		jest.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
		jest.spyOn(API, "checkAuthorization").mockResolvedValue(MockPermissions);

		jest
			.spyOn(API, "templateVersionDynamicParameters")
			.mockImplementation((_versionId, _ownerId, callbacks) => {
				const [mockWebSocket, publisher] = createMockWebSocket("ws://test");

				mockWebSocket.addEventListener("message", (event) => {
					callbacks.onMessage(JSON.parse(event.data));
				});
				mockWebSocket.addEventListener("error", () => {
					callbacks.onError(
						new Error("Connection for dynamic parameters failed."),
					);
				});
				mockWebSocket.addEventListener("close", () => {
					callbacks.onClose();
				});

				publisher.publishOpen(new Event("open"));
				publisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify(MockDynamicParametersResponse),
					}),
				);

				return mockWebSocket;
			});
	});

	afterEach(() => {
		jest.restoreAllMocks();
	});

	describe("WebSocket Integration", () => {
		it("establishes WebSocket connection and receives initial parameters", async () => {
			renderCreateWorkspacePage();

			await waitForLoaderToBeRemoved();

			expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
				MockTemplate.active_version_id,
				MockUserOwner.id,
				expect.objectContaining({
					onMessage: expect.any(Function),
					onError: expect.any(Function),
					onClose: expect.any(Function),
				}),
			);

			await waitFor(() => {
				expect(screen.getByText(/instance type/i)).toBeInTheDocument();
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(screen.getByText("Tags")).toBeInTheDocument();
			});
		});

		it("sends parameter updates via WebSocket when form values change", async () => {
			const [mockWebSocket, publisher] = createMockWebSocket("ws://test");

			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((_versionId, _ownerId, callbacks) => {
					mockWebSocket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});
					mockWebSocket.addEventListener("error", () => {
						callbacks.onError(
							new Error("Connection for dynamic parameters failed."),
						);
					});
					mockWebSocket.addEventListener("close", () => {
						callbacks.onClose();
					});

					publisher.publishOpen(new Event("open"));
					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(MockDynamicParametersResponse),
						}),
					);

					return mockWebSocket;
				});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			const instanceTypeSelect = screen.getByRole("button", {
				name: /instance type/i,
			});
			expect(instanceTypeSelect).toBeInTheDocument();

			await waitFor(async () => {
				await userEvent.click(instanceTypeSelect);
			});

			let mediumOption: Element | null = null;
			await waitFor(() => {
				mediumOption = screen.queryByRole("option", { name: /t3\.medium/i });
				expect(mediumOption).toBeTruthy();
			});

			await waitFor(async () => {
				await userEvent.click(mediumOption!);
			});

			expect(mockWebSocket.send).toHaveBeenCalledWith(
				expect.stringContaining('"instance_type":"t3.medium"'),
			);
		});

		it("handles WebSocket error gracefully", async () => {
			const [mockWebSocket, mockPublisher] = createMockWebSocket("ws://test");

			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((_versionId, _ownerId, callbacks) => {
					mockWebSocket.addEventListener("error", () => {
						callbacks.onError(new Error("Connection failed"));
					});

					return mockWebSocket;
				});

			renderCreateWorkspacePage();

			await waitFor(() => {
				expect(mockPublisher).toBeDefined();
				mockPublisher.publishError(new Event("Connection failed"));
				expect(screen.getByText(/connection failed/i)).toBeInTheDocument();
			});
		});

		it("handles WebSocket close event", async () => {
			const [mockWebSocket, mockPublisher] = createMockWebSocket("ws://test");

			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((_versionId, _ownerId, callbacks) => {
					mockWebSocket.addEventListener("close", () => {
						callbacks.onClose();
					});

					return mockWebSocket;
				});

			renderCreateWorkspacePage();

			await waitFor(() => {
				expect(mockPublisher).toBeDefined();
				mockPublisher.publishClose(new Event("close") as CloseEvent);
				expect(
					screen.getByText(/websocket connection.*unexpectedly closed/i),
				).toBeInTheDocument();
			});
		});

		it("only parameters from latest response are displayed", async () => {
			const [mockWebSocket, mockPublisher] = createMockWebSocket("ws://test");
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((_versionId, _ownerId, callbacks) => {
					mockWebSocket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});

					mockPublisher.publishOpen(new Event("open"));
					mockPublisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify({
								id: 0,
								parameters: [MockDropdownParameter],
								diagnostics: [],
							}),
						}),
					);

					return mockWebSocket;
				});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			const response1: DynamicParametersResponse = {
				id: 1,
				parameters: [MockDropdownParameter],
				diagnostics: [],
			};
			const response2: DynamicParametersResponse = {
				id: 4,
				parameters: [MockSliderParameter],
				diagnostics: [],
			};

			await waitFor(() => {
				mockPublisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response1) }),
				);

				mockPublisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response2) }),
				);
			});

			expect(screen.queryByText("CPU Count")).toBeInTheDocument();
			expect(screen.queryByText("Instance Type")).not.toBeInTheDocument();
		});
	});

	describe("Dynamic Parameter Types", () => {
		it("displays parameter validation errors", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((_versionId, _ownerId, callbacks) => {
					const [mockWebSocket, publisher] = createMockWebSocket("ws://test");

					mockWebSocket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});

					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(MockDynamicParametersResponseWithError),
						}),
					);

					return mockWebSocket;
				});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Validation failed")).toBeInTheDocument();
				expect(
					screen.getByText(
						"The selected instance type is not available in this region",
					),
				).toBeInTheDocument();
			});
		});

		it("displays parameter validation errors for min/max constraints", async () => {
			const mockResponseInitial: DynamicParametersResponse = {
				id: 1,
				parameters: [MockValidationParameter],
				diagnostics: [],
			};

			const mockResponseWithError: DynamicParametersResponse = {
				id: 2,
				parameters: [
					{
						...MockValidationParameter,
						value: { value: "200", valid: false },
						diagnostics: [
							{
								severity: "error",
								summary:
									"Invalid parameter value according to 'validation' block",
								detail: "value 200 is more than the maximum 100",
								extra: {
									code: "",
								},
							},
						],
					},
				],
				diagnostics: [],
			};

			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((_versionId, _ownerId, callbacks) => {
					const [mockWebSocket, publisher] = createMockWebSocket("ws://test");

					mockWebSocket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});

					publisher.publishOpen(new Event("open"));

					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(mockResponseInitial),
						}),
					);

					const originalSend = mockWebSocket.send;
					mockWebSocket.send = jest.fn((data) => {
						originalSend.call(mockWebSocket, data);

						if (typeof data === "string" && data.includes('"200"')) {
							publisher.publishMessage(
								new MessageEvent("message", {
									data: JSON.stringify(mockResponseWithError),
								}),
							);
						}
					});

					return mockWebSocket;
				});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Invalid Parameter")).toBeInTheDocument();
			});

			const numberInput = screen.getByDisplayValue("50");
			expect(numberInput).toBeInTheDocument();

			await waitFor(async () => {
				await userEvent.clear(numberInput);
				await userEvent.type(numberInput, "200");
			});

			await waitFor(() => {
				expect(screen.getByDisplayValue("200")).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(
					screen.getByText(
						"Invalid parameter value according to 'validation' block",
					),
				).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(
					screen.getByText("value 200 is more than the maximum 100"),
				).toBeInTheDocument();
			});

			const errorElement = screen.getByText(
				"value 200 is more than the maximum 100",
			);
			expect(errorElement.closest("div")).toHaveClass(
				"text-content-destructive",
			);
		});
	});

	describe("External Authentication", () => {
		it("displays external auth providers", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(
					screen.getByRole("button", { name: /login with github/i }),
				).toBeInTheDocument();
			});
		});

		it("shows authenticated state for connected providers", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([
					MockTemplateVersionExternalAuthGithubAuthenticated,
				]);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(screen.getByText(/authenticated/i)).toBeInTheDocument();
			});
		});

		it("prevents auto-creation when required external auth is missing", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?mode=auto`,
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(
					screen.getByText(
						/external authentication providers that are not connected/i,
					),
				).toBeInTheDocument();
				expect(
					screen.getByText(/auto-creation has been disabled/i),
				).toBeInTheDocument();
			});
		});
	});

	describe("Auto-creation Mode", () => {
		it("falls back to form mode when auto-creation fails", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([
					MockTemplateVersionExternalAuthGithubAuthenticated,
				]);
			jest
				.spyOn(API, "createWorkspace")
				.mockRejectedValue(new Error("Auto-creation failed"));

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?mode=auto`,
			);

			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			await waitFor(() => {
				expect(screen.getByText("Create workspace")).toBeInTheDocument();
				expect(
					screen.getByRole("button", { name: /create workspace/i }),
				).toBeInTheDocument();
			});
		});
	});

	describe("Form Submission", () => {
		it("creates workspace with correct parameters", async () => {
			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});
			await waitFor(async () => {
				await userEvent.clear(nameInput);
				await userEvent.type(nameInput, "my-test-workspace");
			});

			const createButton = screen.getByRole("button", {
				name: /create workspace/i,
			});
			await waitFor(async () => {
				await userEvent.click(createButton);
			});

			await waitFor(() => {
				expect(API.createWorkspace).toHaveBeenCalledWith(
					"test-user",
					expect.objectContaining({
						name: "my-test-workspace",
						template_version_id: MockTemplate.active_version_id,
						template_id: undefined,
						rich_parameter_values: [
							expect.objectContaining({ name: "instance_type", value: "" }),
							expect.objectContaining({ name: "cpu_count", value: "2" }),
							expect.objectContaining({
								name: "enable_monitoring",
								value: "true",
							}),
							expect.objectContaining({ name: "tags", value: "[]" }),
							expect.objectContaining({ name: "ides", value: "[]" }),
						],
					}),
				);
			});
		});
	});

	describe("URL Parameters", () => {
		it("pre-fills parameters from URL", async () => {
			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?param.instance_type=t3.large&param.cpu_count=4`,
			);
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();
			expect(screen.getByText("CPU Count")).toBeInTheDocument();
		});

		it("uses custom template version when specified", async () => {
			const customVersionId = "custom-version-123";

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?version=${customVersionId}`,
			);

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
					customVersionId,
					MockUserOwner.id,
					expect.any(Object),
				);
			});
		});

		it("pre-fills workspace name from URL", async () => {
			const workspaceName = "my-custom-workspace";

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?name=${workspaceName}`,
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				const nameInput = screen.getByRole("textbox", {
					name: /workspace name/i,
				});
				expect(nameInput).toHaveValue(workspaceName);
			});
		});
	});

	describe("Navigation", () => {
		it("navigates to workspace after successful creation", async () => {
			const { router } = renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});

			await waitFor(async () => {
				await userEvent.clear(nameInput);
				await userEvent.type(nameInput, "my-test-workspace");
			});

			// Submit form
			const createButton = screen.getByRole("button", {
				name: /create workspace/i,
			});
			await waitFor(async () => {
				await userEvent.click(createButton);
			});

			await waitFor(() => {
				expect(router.state.location.pathname).toBe(
					`/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
				);
			});
		});
	});
});
